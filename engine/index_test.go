package engine

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/samber/lo"
)

// rangeIndexFactories liste les implémentations de RangeIndex à tester.
// SortedIndex (4.3, tremplin) et BTree (4.4, structure finale) doivent se
// comporter EXACTEMENT pareil : mêmes tests pour les deux (filet de sécurité).
var rangeIndexFactories = map[string]func() RangeIndex{
	"SortedIndex": func() RangeIndex { return NewSortedIndex() },
	"BTree":       func() RangeIndex { return NewBTree() },
}

func TestRangeIndexBasics(t *testing.T) {
	for name, factory := range rangeIndexFactories {
		t.Run(name, func(t *testing.T) {
			idx := factory()
			idx.Insert("30", "alice")
			idx.Insert("30", "bob") // deux clés partagent la même valeur
			idx.Insert("40", "carol")
			idx.Insert("20", "dave")

			assertKeys(t, idx.Range(OpGT, "25"), []string{"alice", "bob", "carol"})
			assertKeys(t, idx.Range(OpGTE, "30"), []string{"alice", "bob", "carol"})
			assertKeys(t, idx.Range(OpLT, "30"), []string{"dave"})
			assertKeys(t, idx.Range(OpLTE, "30"), []string{"alice", "bob", "dave"})
		})
	}
}

func TestRangeIndexDelete(t *testing.T) {
	for name, factory := range rangeIndexFactories {
		t.Run(name, func(t *testing.T) {
			idx := factory()
			idx.Insert("30", "alice")
			idx.Insert("30", "bob")

			idx.Delete("30", "alice")
			assertKeys(t, idx.Range(OpGTE, "0"), []string{"bob"})

			idx.Delete("30", "bob")
			assertKeys(t, idx.Range(OpGTE, "0"), []string{})
		})
	}
}

func TestRangeIndexOverwrite(t *testing.T) {
	for name, factory := range rangeIndexFactories {
		t.Run(name, func(t *testing.T) {
			idx := factory()
			idx.Insert("30", "alice")
			// alice change de valeur : le moteur retire l'ancienne puis ajoute la nouvelle.
			idx.Delete("30", "alice")
			idx.Insert("50", "alice")

			assertKeys(t, idx.Range(OpLT, "40"), []string{})
			assertKeys(t, idx.Range(OpGTE, "40"), []string{"alice"})
		})
	}
}

// TestRangeIndexRandomized insère/supprime des paires (valeur, clé) au hasard
// et compare le résultat des requêtes range à une implémentation de référence
// "bête" (scan linéaire). C'est le filet de sécurité pour la logique de
// rééquilibrage du B-Tree, trop subtile à vérifier à la main cas par cas.
func TestRangeIndexRandomized(t *testing.T) {
	for name, factory := range rangeIndexFactories {
		t.Run(name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(42))
			idx := factory()
			reference := map[string]string{} // key -> value, la vérité de référence

			values := []string{"5", "10", "15", "20", "25", "30", "35", "40", "45", "50"}

			for i := 0; i < 500; i++ {
				key := fmt.Sprintf("k%d", rng.Intn(30))
				if old, exists := reference[key]; exists && rng.Intn(3) == 0 {
					idx.Delete(old, key)
					delete(reference, key)
					continue
				}
				value := values[rng.Intn(len(values))]
				if old, exists := reference[key]; exists {
					idx.Delete(old, key)
				}
				idx.Insert(value, key)
				reference[key] = value

				op := []FilterOp{OpGT, OpGTE, OpLT, OpLTE}[rng.Intn(4)]
				threshold := values[rng.Intn(len(values))]
				assertKeys(t, idx.Range(op, threshold), bruteForceRange(reference, op, threshold))
			}
		})
	}
}

func bruteForceRange(reference map[string]string, op FilterOp, threshold string) []string {
	thresholdKey := sortKey(threshold)
	return lo.Filter(lo.Keys(reference), func(key string, _ int) bool {
		return matchesRange(sortKey(reference[key]), op, thresholdKey)
	})
}

func assertKeys(t *testing.T, got []string, want []string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEngineGetWhereEquals(t *testing.T) {
	e := newTestEngine(t)
	e.Set("alice", "30")
	e.Set("bob", "30")
	e.Set("carol", "40")

	matches, err := e.GetWhere(OpEquals, "30")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2 (%+v)", len(matches), matches)
	}
}

func TestEngineGetWhereContains(t *testing.T) {
	e := newTestEngine(t)
	e.Set("a", "hello world")
	e.Set("b", "goodbye")

	matches, err := e.GetWhere(OpContains, "hello")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(matches) != 1 || matches[0].Key != "a" {
		t.Fatalf("got %+v, want [{a hello world}]", matches)
	}
}

func TestEngineGetWhereRange(t *testing.T) {
	e := newTestEngine(t)
	e.Set("alice", "30")
	e.Set("bob", "40")
	e.Set("carol", "20")

	matches, err := e.GetWhere(OpGTE, "30")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2 (%+v)", len(matches), matches)
	}
}

func TestEngineIndexUpdatedOnOverwriteAndDelete(t *testing.T) {
	e := newTestEngine(t)
	e.Set("alice", "30")
	e.Set("alice", "99") // écrasement : l'ancienne valeur ne doit plus matcher

	matches, _ := e.GetWhere(OpEquals, "30")
	if len(matches) != 0 {
		t.Fatalf("ancienne valeur encore indexée: %+v", matches)
	}
	matches, _ = e.GetWhere(OpEquals, "99")
	if len(matches) != 1 {
		t.Fatalf("nouvelle valeur non indexée: %+v", matches)
	}

	e.Delete("alice")
	matches, _ = e.GetWhere(OpEquals, "99")
	if len(matches) != 0 {
		t.Fatalf("clé supprimée encore indexée: %+v", matches)
	}
}
