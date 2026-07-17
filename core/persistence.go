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
func (e *GoRedis) Snapshot() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.opBuffer) > 0 {
		if err := e.storage.AppendAOF(e.opBuffer); err != nil {
			return err
		}
		e.opBuffer = nil
		e.bufferCount.Store(0)
	}

	stateCopy := make(map[string]SnapshotEntry, len(e.state))
	for k, v := range e.state {
		stateCopy[k] = SnapshotEntry{Value: v, Timestamp: e.timestamps[k]}
	}
	if err := e.storage.WriteSnapshot(stateCopy); err != nil {
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
	e.rangeIndex = NewBTree()
	e.keyIndex = NewBTree()
	e.timeIndex = NewBTree()
	e.timestamps = make(map[string]int64)

	for key, entry := range snapshot {
		e.setLocked(key, entry.Value, time.Unix(0, entry.Timestamp))
	}
	for _, op := range ops {
		switch op.Type {
		case CmdSet:
			e.setLocked(op.Key, op.Value, time.Unix(0, op.Timestamp))
		case CmdDelete:
			e.deleteLocked(op.Key)
		}
	}
	return nil
}
