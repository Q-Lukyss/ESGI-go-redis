package engine

import (
	"sort"

	"github.com/samber/lo"
)

// SortedIndex est le tremplin de la 4.3 : une slice triée par sortKey +
// recherche dichotomique (sort.Search). Insertion en O(n) (décalage de
// slice), requête en O(log n + résultats). Pas l'archi finale (voir BTree),
// mais elle sert de référence : mêmes tests que BTree (index_test.go).
type SortedIndex struct {
	entries []*indexItem
}

// NewSortedIndex crée un index de range vide.
func NewSortedIndex() *SortedIndex {
	return &SortedIndex{}
}

func (s *SortedIndex) find(sk string) (int, bool) {
	i := sort.Search(len(s.entries), func(i int) bool { return s.entries[i].sortKey >= sk })
	if i < len(s.entries) && s.entries[i].sortKey == sk {
		return i, true
	}
	return i, false
}

func (s *SortedIndex) Insert(value, key string) {
	sk := sortKey(value)
	i, found := s.find(sk)
	if found {
		s.entries[i].keys[key] = struct{}{}
		return
	}
	item := &indexItem{sortKey: sk, value: value, keys: map[string]struct{}{key: {}}}
	s.entries = append(s.entries, nil)
	copy(s.entries[i+1:], s.entries[i:])
	s.entries[i] = item
}

func (s *SortedIndex) Delete(value, key string) {
	sk := sortKey(value)
	i, found := s.find(sk)
	if !found {
		return
	}
	delete(s.entries[i].keys, key)
	if len(s.entries[i].keys) == 0 {
		s.entries = append(s.entries[:i], s.entries[i+1:]...)
	}
}

func (s *SortedIndex) Range(op FilterOp, threshold string) []string {
	thresholdKey := sortKey(threshold)
	var result []string
	for _, entry := range s.entries {
		if matchesRange(entry.sortKey, op, thresholdKey) {
			result = append(result, lo.Keys(entry.keys)...)
		}
	}
	return result
}
