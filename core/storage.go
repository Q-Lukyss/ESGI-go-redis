package core

type Operation struct {
	Type      CommandType `json:"type"`
	Key       string      `json:"key"`
	Value     string      `json:"value,omitempty"`
	Timestamp int64       `json:"timestamp,omitempty"` // unix nano ; absent (0) sur les anciennes entrées AOF
	ExpiresAt int64       `json:"expiresAt,omitempty"` // unix nano ; 0 = pas de TTL
}

// SnapshotEntry porte le timestamp de dernière écriture avec la valeur.
// Sans ça, un Restore() horodaterait toutes les clés issues du snapshot à
// l'instant du restart (perte de la chronologie réelle) — critique pour
// timeIndex/mode "Activity", et pour qu'un jeu de données seedé avec des
// timestamps répartis dans le temps garde son sens après un redémarrage.
type SnapshotEntry struct {
	Value     string `json:"value"`
	Timestamp int64  `json:"timestamp"`
	ExpiresAt int64  `json:"expiresAt,omitempty"` // unix nano ; 0 = pas de TTL
}

// Storage découple le moteur de son support physique. Le moteur ne sait
// pas s'il écrit dans des fichiers, une base, ou de la mémoire : il ne
// connaît que ce contrat. Ça permet de changer de support plus tard
// (ex: WASM/OPFS) sans toucher au moteur.
type Storage interface {
	AppendAOF(ops []Operation) error
	ReadAOF() ([]Operation, error)
	ClearAOF() error
	WriteSnapshot(state map[string]SnapshotEntry) error
	ReadSnapshot() (map[string]SnapshotEntry, error)
}
