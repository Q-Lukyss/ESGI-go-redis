package core

import (
	"fmt"
	"sync"
	"time"
)

// GoRedis est le moteur : le state vivant en RAM, avec tout ce qu'il faut
type GoRedis struct {
	mu sync.Mutex

	state map[string]string

	equalsIndex map[string]map[string]struct{} // valeur -> ensemble de clés (4.1)
	rangeIndex  RangeIndex                     // pour >, >=, <, <=

	storage          Storage
	opBuffer         []Operation
	flushInterval    time.Duration
	snapshotInterval time.Duration
}

// NewGoRedis construit un moteur prêt à l'emploi, avec un state vide.
func NewGoRedis(storage Storage, flushInterval, snapshotInterval time.Duration) *GoRedis {
	return &GoRedis{
		state:            make(map[string]string),
		equalsIndex:      make(map[string]map[string]struct{}),
		rangeIndex:       NewBTree(),
		storage:          storage,
		opBuffer:         nil,
		flushInterval:    flushInterval,
		snapshotInterval: snapshotInterval,
	}
}

// Set pose une valeur pour une clé (écrase si déjà présente).
func (e *GoRedis) Set(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setLocked(key, value)
	e.recordOpLocked(Operation{Type: CmdSet, Key: key, Value: value})
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
	e.recordOpLocked(Operation{Type: CmdDelete, Key: key})
	return nil
}

// setLocked applique un SET au state + index, sans toucher au buffer ni au verrou.
// Appelée aussi bien par Set() (nouvelle écriture) que par Restore() (rejeu).
func (e *GoRedis) setLocked(key, value string) {
	if old, ok := e.state[key]; ok {
		e.removeFromIndexesLocked(key, old)
	}
	e.state[key] = value
	e.addToIndexesLocked(key, value)
}

// deleteLocked applique un DELETE au state + index, sans toucher au buffer ni au verrou.
func (e *GoRedis) deleteLocked(key string) {
	old := e.state[key]
	delete(e.state, key)
	e.removeFromIndexesLocked(key, old)
}

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

// ExecuteString parse une ligne brute puis l'exécute, en propageant l'erreur de parsing.
func (e *GoRedis) ExecuteString(line string) (any, error) {
	cmd, err := ParseCommand(line)
	if err != nil {
		return nil, err
	}
	return e.Execute(cmd)
}
