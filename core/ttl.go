package core

import "time"

// SetWithTTL pose une valeur avec un TTL explicite en secondes. ttlSeconds
// <= 0 retombe sur defaultTTLSeconds (constante configurable, cf.
// NewGoRedis) : 0 par défaut, donc un SET sans EX reste persistant, comme
// dans Redis — le mécanisme n'existe que pour permettre de forcer un TTL
// global sans toucher au protocole si un jour le besoin s'en présente.
func (e *GoRedis) SetWithTTL(key, value string, ttlSeconds int64) {
	ttl := ttlSeconds
	if ttl <= 0 {
		ttl = e.defaultTTLSeconds
	}
	if ttl <= 0 {
		e.Set(key, value)
		return
	}
	e.SetEx(key, value, ttl)
}

// SetEx pose une valeur avec une expiration explicite, ttlSeconds secondes
// dans le futur (SET <key> "<value>" EX <ttlSeconds>).
func (e *GoRedis) SetEx(key, value string, ttlSeconds int64) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second).UnixNano()
	e.setAtWithExpiry(key, value, now, expiresAt)
}

// setExpiryLocked pose ou efface l'expiration d'une clé, sous verrou.
// expiresAt <= 0 efface toute expiration précédente (SET sans EX écrase le
// TTL, comme dans Redis).
func (e *GoRedis) setExpiryLocked(key string, expiresAt int64) {
	if expiresAt <= 0 {
		delete(e.expireAt, key)
		return
	}
	e.expireAt[key] = expiresAt
}

// isExpiredLocked indique si une clé a un TTL dépassé, sous verrou.
func (e *GoRedis) isExpiredLocked(key string) bool {
	exp, ok := e.expireAt[key]
	return ok && time.Now().UnixNano() >= exp
}

// expireKeyLocked supprime réellement une clé expirée : state, index, et
// persistée comme une suppression (le prochain Flush/Snapshot l'écrit dans
// l'AOF), exactement comme un DELETE explicite. Appelée sous verrou, par le
// GET paresseux (isExpiredLocked) et par le balayage périodique.
func (e *GoRedis) expireKeyLocked(key string) {
	e.deleteLocked(key)
	e.recordOpLocked(Operation{Type: CmdDelete, Key: key, Timestamp: time.Now().UnixNano()}, false)
}

// RunExpirySweepLoop balaie périodiquement les clés expirées non encore
// accédées (le GET paresseux ne suffit pas à lui seul : une clé jamais
// relue après son expiration resterait en mémoire pour toujours). Ne fait
// rien si expirySweepInterval <= 0 (balayage désactivé). Bloque jusqu'à ce
// que stop soit fermé — à lancer dans sa propre goroutine, comme
// RunFlushLoop/RunSnapshotLoop.
func (e *GoRedis) RunExpirySweepLoop(stop <-chan struct{}) {
	if e.expirySweepInterval <= 0 {
		return
	}
	ticker := time.NewTicker(e.expirySweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			e.sweepExpired()
		case <-stop:
			return
		}
	}
}

// sweepExpired supprime pour de bon toutes les clés dont le TTL est dépassé.
func (e *GoRedis) sweepExpired() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixNano()
	for key, exp := range e.expireAt {
		if now >= exp {
			e.expireKeyLocked(key)
		}
	}
}
