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

	storage          Storage
	opBuffer         []Operation
	flushInterval    time.Duration
	snapshotInterval time.Duration

	stateCount  atomic.Int64 // miroir de len(state), lisible sans prendre mu
	bufferCount atomic.Int64 // miroir de len(opBuffer), lisible sans prendre mu
	changes     chan ChangeEvent
}

func NewGoRedis(storage Storage, flushInterval, snapshotInterval time.Duration) *GoRedis {
	return &GoRedis{
		state:            make(map[string]string),
		equalsIndex:      make(map[string]map[string]struct{}),
		rangeIndex:       NewBTree(),
		keyIndex:         NewBTree(),
		timeIndex:        NewBTree(),
		timestamps:       make(map[string]int64),
		storage:          storage,
		opBuffer:         nil,
		flushInterval:    flushInterval,
		snapshotInterval: snapshotInterval,
		changes:          make(chan ChangeEvent, changesBufferSize),
	}
}

// Set pose une valeur pour une clé (écrase si déjà présente), horodatée à
// l'instant présent.
func (e *GoRedis) Set(key, value string) {
	e.SetAt(key, value, time.Now())
}

// SetAt pose une valeur pour une clé avec un timestamp explicite. Utilisée
// par Set (avec l'heure courante) et par les scripts de seed, qui ont
// besoin de répartir des timestamps dans le temps pour peupler une démo.
func (e *GoRedis) SetAt(key, value string, ts time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	existed := e.setLocked(key, value, ts)
	e.recordOpLocked(Operation{Type: CmdSet, Key: key, Value: value, Timestamp: ts.UnixNano()}, !existed)
}

// Get lit une valeur ; erreur si la clé est absente.
func (e *GoRedis) Get(key string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
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
		e.Set(cmd.Key, cmd.Value)
		return nil, nil
	case CmdGet:
		return e.Get(cmd.Key)
	case CmdDelete:
		return nil, e.Delete(cmd.Key)
	case CmdGetWhere:
		return e.GetWhere(cmd.FilterOp, cmd.FilterValue)
	default:
		return nil, fmt.Errorf("type de commande non géré: %s", cmd.Type)
	}
}
