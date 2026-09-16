package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// GoRedis est le moteur : le state vivant en RAM, avec tout ce qu'il faut
type GoRedis struct {
	mu sync.Mutex

	state map[string]string

	equalsIndex map[string]map[string]struct{}
	rangeIndex  RangeIndex       // pour >, >=, <, <=
	keyIndex    *BTree           // parcours trié par clé, paginé (scroll infini, mode "Browse")
	timeIndex   *BTree           // parcours trié par date de dernière écriture, paginé (mode "Activity")
	timestamps  map[string]int64 // dernier timestamp (unix nano) connu par clé, pour retirer proprement de timeIndex
	expireAt    map[string]int64 // unix nano d'expiration par clé ; absent = pas de TTL (cf. core/ttl.go)

	storage             Storage
	opBuffer            []Operation
	flushInterval       time.Duration
	snapshotInterval    time.Duration
	expirySweepInterval time.Duration
	defaultTTLSeconds   int64 // TTL appliqué à un SET sans EX explicite ; <= 0 = désactivé (comportement Redis standard)
	btreeDegree         int   // ordre du B-Tree, retenu pour que Restore() recrée les index au même ordre

	stateCount  atomic.Int64 // miroir de len(state), lisible sans prendre mu
	bufferCount atomic.Int64 // miroir de len(opBuffer), lisible sans prendre mu
	changes     chan ChangeEvent
}

// NewGoRedis construit le moteur à partir de cfg (cf. Config, DefaultConfig) :
// aucune valeur de tuning n'est câblée en dur ici, tout vient de l'appelant.
func NewGoRedis(storage Storage, cfg Config) *GoRedis {
	return &GoRedis{
		state:               make(map[string]string),
		equalsIndex:         make(map[string]map[string]struct{}),
		rangeIndex:          NewBTree(cfg.BTreeDegree),
		keyIndex:            NewBTree(cfg.BTreeDegree),
		timeIndex:           NewBTree(cfg.BTreeDegree),
		timestamps:          make(map[string]int64),
		expireAt:            make(map[string]int64),
		storage:             storage,
		opBuffer:            nil,
		flushInterval:       cfg.FlushInterval,
		snapshotInterval:    cfg.SnapshotInterval,
		expirySweepInterval: cfg.ExpirySweepInterval,
		defaultTTLSeconds:   cfg.DefaultTTLSeconds,
		btreeDegree:         cfg.BTreeDegree,
		changes:             make(chan ChangeEvent, cfg.ChangesBufferSize),
	}
}

// Set pose une valeur pour une clé (écrase si déjà présente), horodatée à
// l'instant présent.
func (e *GoRedis) Set(key, value string) {
	e.SetAt(key, value, time.Now())
}

// SetAt pose une valeur pour une clé avec un timestamp explicite, sans TTL.
// Utilisée par Set (avec l'heure courante) et par les scripts de seed, qui
// ont besoin de répartir des timestamps dans le temps pour peupler une démo.
func (e *GoRedis) SetAt(key, value string, ts time.Time) {
	e.setAtWithExpiry(key, value, ts, 0)
}

// setAtWithExpiry est le point d'entrée commun à tous les SET (avec ou sans
// TTL) : applique state+index, pose/efface l'expiration de la clé, puis
// journalise. expiresAt <= 0 signifie "pas de TTL" (efface une expiration
// précédente, comme un SET sans EX écrase le TTL dans Redis).
func (e *GoRedis) setAtWithExpiry(key, value string, ts time.Time, expiresAt int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	existed := e.setLocked(key, value, ts)
	e.setExpiryLocked(key, expiresAt)
	e.recordOpLocked(Operation{Type: CmdSet, Key: key, Value: value, Timestamp: ts.UnixNano(), ExpiresAt: expiresAt}, !existed)
}

// Get lit une valeur ; erreur si la clé est absente ou expirée (expiration
// lazy : une clé dont le TTL est dépassé est supprimée pour de bon au
// premier accès, cf. core/ttl.go).
func (e *GoRedis) Get(key string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.isExpiredLocked(key) {
		e.expireKeyLocked(key)
	}
	value, ok := e.state[key]
	if !ok {
		return "", fmt.Errorf("clé absente: %s", key)
	}
	return value, nil
}

// Delete supprime une clé ; erreur si elle est absente.
func (e *GoRedis) Delete(key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.state[key]; !ok {
		return fmt.Errorf("clé absente: %s", key)
	}
	e.deleteLocked(key)
	e.recordOpLocked(Operation{Type: CmdDelete, Key: key, Timestamp: time.Now().UnixNano()}, false)
	return nil
}

// setLocked applique un SET au state + index, sans toucher au buffer ni au verrou.
// Appelée aussi bien par SetAt() (nouvelle écriture) que par Restore() (rejeu).
// Renvoie existed=true si la clé remplaçait une valeur précédente (utile aux
// transports pour distinguer une simple mise à jour de valeur — patchable en
// place — d'une insertion, qui change la position dans keyIndex/timeIndex).
func (e *GoRedis) setLocked(key, value string, ts time.Time) (existed bool) {
	if old, ok := e.state[key]; ok {
		e.removeFromIndexesLocked(key, old)
		existed = true
	}
	e.state[key] = value
	e.addToIndexesLocked(key, value, ts.UnixNano())
	e.stateCount.Store(int64(len(e.state)))
	return existed
}

// deleteLocked applique un DELETE au state + index, sans toucher au buffer ni au verrou.
func (e *GoRedis) deleteLocked(key string) {
	old := e.state[key]
	delete(e.state, key)
	delete(e.expireAt, key)
	e.removeFromIndexesLocked(key, old)
	e.stateCount.Store(int64(len(e.state)))
}

// StateCount et BufferCount donnent une vue instantanée des tailles du store
// et de la file d'écritures en attente, sans jamais prendre mu — pensé pour
// un ticker de stats (WS/wasmbridge) qui ne doit pas entrer en contention
// avec le chemin d'écriture.
func (e *GoRedis) StateCount() int64  { return e.stateCount.Load() }
func (e *GoRedis) BufferCount() int64 { return e.bufferCount.Load() }

// Execute exécute une commande déjà parsée et aiguille vers la bonne méthode.
func (e *GoRedis) Execute(cmd Command) (any, error) {
	switch cmd.Type {
	case CmdSet:
		e.SetWithTTL(cmd.Key, cmd.Value, cmd.ExpireSeconds)
		return nil, nil
	case CmdGet:
		return e.Get(cmd.Key)
	case CmdDelete:
		return nil, e.Delete(cmd.Key)
	case CmdGetWhere:
		return e.GetWhere(cmd.FilterOp, cmd.FilterValue)
	case CmdFlushAll:
		return nil, e.Clear()
	default:
		return nil, fmt.Errorf("type de commande non géré: %s", cmd.Type)
	}
}
