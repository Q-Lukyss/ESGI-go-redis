package core

type Operation struct {
	Type      CommandType `json:"type"`
	Key       string      `json:"key"`
	Value     string      `json:"value,omitempty"`
	Timestamp int64       `json:"timestamp,omitempty"` // unix nano ; absent (0) sur les anciennes entrées AOF
}

// Storage découple le moteur de son support physique. Le moteur ne sait
// pas s'il écrit dans des fichiers, une base, ou de la mémoire : il ne
// connaît que ce contrat. Ça permet de changer de support plus tard
// (ex: WASM/IndexedDB) sans toucher au moteur.
type Storage interface {
	AppendAOF(ops []Operation) error
	ReadAOF() ([]Operation, error)
	ClearAOF() error
	WriteSnapshot(state map[string]string) error
	ReadSnapshot() (map[string]string, error)
}
