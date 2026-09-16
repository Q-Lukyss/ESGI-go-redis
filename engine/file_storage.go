package engine

import (
	"bufio"
	"encoding/json"
	"os"
)

// FileStorage est l'implémentation native de Storage : un fichier AOF
// (ouvert en append) et un fichier snapshot (JSON, réécrit atomiquement
// via fichier temporaire + rename).
type FileStorage struct {
	aofPath      string
	snapshotPath string
}

// NewFileStorage crée un Storage basé sur deux fichiers du disque.
func NewFileStorage(aofPath, snapshotPath string) *FileStorage {
	return &FileStorage{aofPath: aofPath, snapshotPath: snapshotPath}
}

func (s *FileStorage) AppendAOF(ops []Operation) error {
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

func (s *FileStorage) ReadAOF() ([]Operation, error) {
	file, err := os.Open(s.aofPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ops []Operation
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var op Operation
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

func (s *FileStorage) ClearAOF() error {
	return os.WriteFile(s.aofPath, nil, 0644)
}

func (s *FileStorage) WriteSnapshot(state map[string]string) error {
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

func (s *FileStorage) ReadSnapshot() (map[string]string, error) {
	data, err := os.ReadFile(s.snapshotPath)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return make(map[string]string), nil
	}
	state := make(map[string]string)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}
