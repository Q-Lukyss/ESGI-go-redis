package core

import "fmt"

// BrowseMode sélectionne l'index utilisé par Browse : tri alphabétique par
// clé (mode "Browse" du dashboard) ou chronologique par dernière écriture
// (mode "Activity").
type BrowseMode string

const (
	BrowseByKey  BrowseMode = "key"
	BrowseByTime BrowseMode = "time"
)

// Browse pagine l'ensemble du store par curseur, triée selon mode. Conçue
// pour le scroll infini : ne matérialise jamais plus que limit entrées,
// quelle que soit la taille du store (cf. BTree.RangeFrom).
func (e *GoRedis) Browse(mode BrowseMode, cursor string, limit int) (matches []Match, nextCursor string, hasMore bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var index *BTree
	switch mode {
	case BrowseByKey:
		index = e.keyIndex
	case BrowseByTime:
		index = e.timeIndex
	default:
		return nil, "", false, fmt.Errorf("mode de parcours non supporté: %s", mode)
	}

	keys, nextCursor, hasMore := index.RangeFrom(cursor, limit)
	return e.matchesFromOrderedKeysLocked(keys), nextCursor, hasMore, nil
}
