package core

import (
	"fmt"

	"github.com/samber/lo"
)

// BrowseMode sélectionne l'index utilisé par Browse : tri alphabétique par
// clé (mode "Browse" du dashboard) ou chronologique par dernière écriture
// (mode "Activity").
type BrowseMode string

const (
	BrowseByKey  BrowseMode = "key"
	BrowseByTime BrowseMode = "time"
)

// BrowseEntry est une entrée renvoyée par Browse : contrairement à Match
// (GetWhere), elle porte aussi le timestamp de dernière écriture — utile à
// l'affichage ("modifié il y a 3 min") et pour qu'un client WS puisse
// déclarer sa fenêtre visible en mode "time" sans connaître l'encodage
// interne des index.
type BrowseEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Timestamp est en unix-nano (~1.7e18) : au-delà de 2^53, un JSON
	// number perdrait en précision une fois désérialisé en JS/TS (IEEE754
	// double). ",string" force un encodage en chaîne pour un aller-retour
	// exact ; le front le lit via BigInt.
	Timestamp int64 `json:"timestamp,string"`
}

// Browse pagine l'ensemble du store par curseur, triée selon mode. Conçue
// pour le scroll infini : ne matérialise jamais plus que limit entrées,
// quelle que soit la taille du store (cf. BTree.RangeFrom).
func (e *GoRedis) Browse(mode BrowseMode, cursor string, limit int) (entries []BrowseEntry, nextCursor string, hasMore bool, err error) {
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
	entries = lo.Map(keys, func(key string, _ int) BrowseEntry {
		return BrowseEntry{Key: key, Value: e.state[key], Timestamp: e.timestamps[key]}
	})
	return entries, nextCursor, hasMore, nil
}
