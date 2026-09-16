package engine

import (
	"fmt"
	"sync"
	"time"
)

// Engine est le moteur : le state vivant en RAM, plus tout ce qu'il faut
// pour le rendre durable (Phase 2) et interrogeable (Phase 4).
// Un seul verrou protège state + index + buffer d'écritures : c'est plus
// simple et suffisant à notre échelle (pas de lock séparé par structure).
type Engine struct {
	mu sync.Mutex

	state map[string]string

	equalsIndex map[string]map[string]struct{} // valeur -> ensemble de clés (4.1)
	rangeIndex  RangeIndex                      // pour >, >=, <, <= (4.3 / 4.4)

	storage          Storage
	opBuffer         []Operation
	flushInterval    time.Duration
	snapshotInterval time.Duration
}

// NewEngine construit un moteur prêt à l'emploi, avec un state vide.
func NewEngine(storage Storage, flushInterval, snapshotInterval time.Duration) *Engine {
	return &Engine{
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
func (e *Engine) Set(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setLocked(key, value)
	e.recordOpLocked(Operation{Type: CmdSet, Key: key, Value: value})
}

// Get lit une valeur ; erreur si la clé est absente.
func (e *Engine) Get(key string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	value, ok := e.state[key]
	if !ok {
		return "", fmt.Errorf("clé absente: %s", key)
	}
	return value, nil
}

// Delete supprime une clé ; erreur si elle est absente.
func (e *Engine) Delete(key string) error {
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
func (e *Engine) setLocked(key, value string) {
	if old, ok := e.state[key]; ok {
		e.removeFromIndexesLocked(key, old)
	}
	e.state[key] = value
	e.addToIndexesLocked(key, value)
}

// deleteLocked applique un DELETE au state + index, sans toucher au buffer ni au verrou.
func (e *Engine) deleteLocked(key string) {
	old := e.state[key]
	delete(e.state, key)
	e.removeFromIndexesLocked(key, old)
}

// Execute exécute une commande déjà parsée et aiguille vers la bonne méthode.
func (e *Engine) Execute(cmd Command) (any, error) {
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
func (e *Engine) ExecuteString(line string) (any, error) {
	cmd, err := ParseCommand(line)
	if err != nil {
		return nil, err
	}
	return e.Execute(cmd)
}
