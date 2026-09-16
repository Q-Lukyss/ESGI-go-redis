package core

import "time"

// recordOpLocked ajoute une opération à la file d'attente, sous verrou.
// Appelée depuis les méthodes d'écriture (Set/Delete) — jamais depuis Restore,
// pour ne pas re-journaliser des opérations déjà présentes sur disque.
// isNew indique si un SET a créé la clé (sans objet pour un DELETE) ; voir
// ChangeEvent.IsNew.
func (e *GoRedis) recordOpLocked(op Operation, isNew bool) {
	e.opBuffer = append(e.opBuffer, op)
	e.bufferCount.Store(int64(len(e.opBuffer)))
	e.publishChangeLocked(op, isNew)
}

// Flush vide la file d'opérations en attente vers l'AOF. Ne fait rien si la
// file est vide. Le verrou garantit qu'un seul flush a lieu à la fois.
func (e *GoRedis) Flush() error {
	e.mu.Lock()
	pending := e.opBuffer
	e.opBuffer = nil
	e.bufferCount.Store(0)
	e.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}
	return e.storage.AppendAOF(pending)
}

// RunFlushLoop déclenche Flush toutes les flushInterval, jusqu'à ce que stop soit fermé.
func (e *GoRedis) RunFlushLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			e.Flush()
		case <-stop:
			return
		}
	}
}

// Snapshot écrit une photo complète du state sur disque, puis vide l'AOF :
// les opérations passées sont désormais toutes couvertes par la photo
// (compaction). Toute opération encore en attente dans le buffer est aussi
// flushée d'abord, pour ne rien perdre.
//
// Le verrou est relâché AVANT les appels à Storage (comme Flush) : pour le
// backend WASM, ces appels attendent une Promise JS (OPFS), donc yield vers
// la boucle d'évènements du navigateur. Si e.mu restait tenu pendant ce
// temps, le moindre message entrant nécessitant aussi e.mu (ex. "browse")
// se bloquerait dessus — et comme il tournerait lui-même dans un callback
// JS synchrone, la boucle d'évènements ne pourrait jamais revenir traiter
// la Promise qui devait débloquer Snapshot : interblocage. Bug réel
// rencontré (seed jamais terminé) et corrigé ici.
func (e *GoRedis) Snapshot() error {
	e.mu.Lock()
	pending := e.opBuffer
	e.opBuffer = nil
	e.bufferCount.Store(0)

	stateCopy := make(map[string]SnapshotEntry, len(e.state))
	for k, v := range e.state {
		stateCopy[k] = SnapshotEntry{Value: v, Timestamp: e.timestamps[k], ExpiresAt: e.expireAt[k]}
	}
	e.mu.Unlock()

	if len(pending) > 0 {
		if err := e.storage.AppendAOF(pending); err != nil {
			return err
		}
	}
	if err := e.storage.WriteSnapshot(stateCopy); err != nil {
		return err
	}
	return e.storage.ClearAOF()
}

// Clear vide entièrement le store (state, index, buffer, TTL) et persiste
// immédiatement l'état vide (nouveau snapshot + AOF vidé) — l'équivalent
// d'un FLUSHALL Redis. Même raisonnement que Snapshot() pour l'ordre
// verrou/Storage : e.mu est relâché avant les appels à Storage.
func (e *GoRedis) Clear() error {
	e.mu.Lock()
	e.state = make(map[string]string)
	e.equalsIndex = make(map[string]map[string]struct{})
	e.rangeIndex = NewBTree(e.btreeDegree)
	e.keyIndex = NewBTree(e.btreeDegree)
	e.timeIndex = NewBTree(e.btreeDegree)
	e.timestamps = make(map[string]int64)
	e.expireAt = make(map[string]int64)
	e.opBuffer = nil
	e.stateCount.Store(0)
	e.bufferCount.Store(0)
	e.mu.Unlock()

	if err := e.storage.WriteSnapshot(make(map[string]SnapshotEntry)); err != nil {
		return err
	}
	return e.storage.ClearAOF()
}

// RunSnapshotLoop déclenche Snapshot toutes les snapshotInterval, jusqu'à ce que stop soit fermé.
func (e *GoRedis) RunSnapshotLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(e.snapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			e.Snapshot()
		case <-stop:
			return
		}
	}
}

// Restore reconstitue le state : on charge le snapshot, puis on rejoue
// par-dessus chaque opération de l'AOF (les écritures survenues après la
// dernière photo). Les opérations rejouées ne sont pas re-journalisées.
// Pour finir, les clés dont le TTL a expiré pendant que le moteur était
// arrêté sont supprimées pour de bon (et journalisées comme un DELETE) :
// sans ça, une clé expirée avant même le redémarrage resterait visible
// jusqu'au premier GET ou au premier passage du balayage périodique.
func (e *GoRedis) Restore() error {
	snapshot, err := e.storage.ReadSnapshot()
	if err != nil {
		return err
	}
	ops, err := e.storage.ReadAOF()
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.state = make(map[string]string)
	e.equalsIndex = make(map[string]map[string]struct{})
	e.rangeIndex = NewBTree(e.btreeDegree)
	e.keyIndex = NewBTree(e.btreeDegree)
	e.timeIndex = NewBTree(e.btreeDegree)
	e.timestamps = make(map[string]int64)
	e.expireAt = make(map[string]int64)

	for key, entry := range snapshot {
		e.setLocked(key, entry.Value, time.Unix(0, entry.Timestamp))
		e.setExpiryLocked(key, entry.ExpiresAt)
	}
	for _, op := range ops {
		switch op.Type {
		case CmdSet:
			e.setLocked(op.Key, op.Value, time.Unix(0, op.Timestamp))
			e.setExpiryLocked(op.Key, op.ExpiresAt)
		case CmdDelete:
			e.deleteLocked(op.Key)
		}
	}

	now := time.Now().UnixNano()
	for key, exp := range e.expireAt {
		if now >= exp {
			e.expireKeyLocked(key)
		}
	}
	return nil
}
