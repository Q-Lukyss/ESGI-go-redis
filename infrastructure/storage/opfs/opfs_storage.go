//go:build js && wasm

// Package opfs implémente core.Storage sur l'Origin Private File System du
// navigateur (AOF en append, snapshot JSON), pour le backend WASM. Miroir
// d'infrastructure/storage/file (le backend serveur) : même contrat
// core.Storage, adaptateur différent — le moteur (core/) ne voit aucune
// différence entre les deux.
package opfs

import (
	"bufio"
	"bytes"
	"encoding/json"

	"ESGI-go-redis/core"
)

// Storage est l'implémentation de core.Storage sur OPFS.
type Storage struct {
	aofName      string
	snapshotName string
}

// New crée un Storage adossé à deux fichiers OPFS (à la racine de l'origine).
func New(aofName, snapshotName string) *Storage {
	return &Storage{aofName: aofName, snapshotName: snapshotName}
}

func (s *Storage) AppendAOF(ops []core.Operation) error {
	if len(ops) == 0 {
		return nil
	}
	handle, err := getSyncAccessHandle(s.aofName)
	if err != nil {
		return err
	}
	defer handle.Call("close")

	var buf bytes.Buffer
	for _, op := range ops {
		line, err := json.Marshal(op)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	appendBytes(handle, buf.Bytes())
	return nil
}

func (s *Storage) ReadAOF() ([]core.Operation, error) {
	handle, err := getSyncAccessHandle(s.aofName)
	if err != nil {
		return nil, err
	}
	defer handle.Call("close")

	var ops []core.Operation
	scanner := bufio.NewScanner(bytes.NewReader(readAll(handle)))
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
	return ops, scanner.Err()
}

func (s *Storage) ClearAOF() error {
	handle, err := getSyncAccessHandle(s.aofName)
	if err != nil {
		return err
	}
	defer handle.Call("close")
	writeAll(handle, nil)
	return nil
}

func (s *Storage) WriteSnapshot(state map[string]core.SnapshotEntry) error {
	handle, err := getSyncAccessHandle(s.snapshotName)
	if err != nil {
		return err
	}
	defer handle.Call("close")

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	writeAll(handle, data)
	return nil
}

func (s *Storage) ReadSnapshot() (map[string]core.SnapshotEntry, error) {
	handle, err := getSyncAccessHandle(s.snapshotName)
	if err != nil {
		return nil, err
	}
	defer handle.Call("close")

	data := readAll(handle)
	if len(data) == 0 {
		return make(map[string]core.SnapshotEntry), nil
	}
	state := make(map[string]core.SnapshotEntry)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}
