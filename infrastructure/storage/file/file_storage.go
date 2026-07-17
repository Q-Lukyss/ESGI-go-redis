// Package file implémente core.Storage sur le système de fichiers natif
// (AOF en append, snapshot JSON). C'est l'adaptateur de stockage utilisé
// par le backend serveur (cmd/server, cmd/seed) ; le backend WASM utilise
// son propre adaptateur (infrastructure/storage/opfs), tous deux
// interchangeables derrière core.Storage.
package file

import (
	"bufio"
	"encoding/json"
	"os"

	"ESGI-go-redis/core"
)

// Storage est l'implémentation de core.Storage avec un fichier AOF.
type Storage struct {
	aofPath      string
	snapshotPath string
}

// New crée un Storage basé sur deux fichiers du disque.
func New(aofPath, snapshotPath string) *Storage {
	return &Storage{aofPath: aofPath, snapshotPath: snapshotPath}
}

func (s *Storage) AppendAOF(ops []core.Operation) error {
	if len(ops) == 0 {
		return nil
	}
	file, err := os.OpenFile(s.aofPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, op := range ops {
		line, err := json.Marshal(op)
		if err != nil {
			return err
		}
		if _, err := writer.Write(line); err != nil {
			return err
		}
		if _, err := writer.WriteString("\n"); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func (s *Storage) ReadAOF() ([]core.Operation, error) {
	file, err := os.Open(s.aofPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ops []core.Operation
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var op core.Operation
		if err := json.Unmarshal(line, &op); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ops, nil
}

func (s *Storage) ClearAOF() error {
	return os.WriteFile(s.aofPath, nil, 0644)
}

func (s *Storage) WriteSnapshot(state map[string]core.SnapshotEntry) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmpPath := s.snapshotPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	// Sur Linux, os.Rename est atomique et évite toute corruption en cas de
	// crash pendant l'écriture. Sur Windows ce n'est pas garanti, mais c'est
	// tout de même bien mieux que d'écrire directement sur le fichier final.
	return os.Rename(tmpPath, s.snapshotPath)
}

func (s *Storage) ReadSnapshot() (map[string]core.SnapshotEntry, error) {
	data, err := os.ReadFile(s.snapshotPath)
	if os.IsNotExist(err) {
		return make(map[string]core.SnapshotEntry), nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return make(map[string]core.SnapshotEntry), nil
	}
	state := make(map[string]core.SnapshotEntry)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}
