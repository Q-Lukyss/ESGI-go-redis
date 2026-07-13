package engine

// Operation représente une écriture (SET ou DELETE) telle qu'elle est
// journalisée dans l'AOF : assez d'information pour la rejouer telle quelle.
type Operation struct {
	Type  CommandType `json:"type"`
	Key   string      `json:"key"`
	Value string      `json:"value,omitempty"`
}

// Storage découple le moteur de son support physique. Le moteur ne sait
// pas s'il écrit dans des fichiers, une base, ou de la mémoire : il ne
// connaît que ce contrat. Ça permet de changer de support plus tard
// (ex: WASM/IndexedDB) sans toucher au moteur.
type Storage interface {
	// AppendAOF ajoute des opérations à la fin du journal (append-only).
	AppendAOF(ops []Operation) error
	// ReadAOF relit le journal dans l'ordre d'écriture.
	ReadAOF() ([]Operation, error)
	// ClearAOF vide le journal (utilisé après un snapshot : compaction).
	ClearAOF() error
	// WriteSnapshot écrit une photo complète du state.
	WriteSnapshot(state map[string]string) error
	// ReadSnapshot relit la dernière photo du state (map vide si aucune).
	ReadSnapshot() (map[string]string, error)
}
