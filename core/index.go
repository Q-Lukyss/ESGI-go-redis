package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
)

// Match est une entrée clé/valeur renvoyée par un GET WHERE.
type Match struct {
	Key   string
	Value string
}

// RangeIndex maintient un index ordonné valeur -> clés, pour répondre aux
// requêtes range (>, >=, <, <=). SortedIndex (4.3, tremplin) et BTree
// (4.4, structure finale) implémentent toutes les deux cette interface,
// et partagent donc les mêmes tests.
// Ces implémentations ne sont pas thread-safe par elles-mêmes : c'est
// l'Engine qui garantit un accès sous verrou.
type RangeIndex interface {
	Insert(value, key string)
	Delete(value, key string)
	// Range renvoie les clés dont la valeur satisfait "valeur <op> threshold".
	Range(op FilterOp, threshold string) []string
}

// indexItem est l'entrée commune aux implémentations de RangeIndex : une
// valeur avec sa clé de tri, et l'ensemble des clés du store qui la portent
// (plusieurs clés peuvent partager la même valeur, ex. plusieurs age=30).
type indexItem struct {
	sortKey string
	value   string
	keys    map[string]struct{}
}

// matchesRange évalue "sortKey <op> thresholdKey" sur des clés de tri déjà normalisées.
func matchesRange(itemSortKey string, op FilterOp, thresholdKey string) bool {
	switch op {
	case OpGT:
		return itemSortKey > thresholdKey
	case OpGTE:
		return itemSortKey >= thresholdKey
	case OpLT:
		return itemSortKey < thresholdKey
	case OpLTE:
		return itemSortKey <= thresholdKey
	default:
		return false
	}
}

// addToIndexesLocked maintient l'index inversé (equals) et l'index de range
// à jour à chaque écriture. Appelée sous verrou par setLocked.
func (e *Engine) addToIndexesLocked(key, value string) {
	if e.equalsIndex[value] == nil {
		e.equalsIndex[value] = make(map[string]struct{})
	}
	e.equalsIndex[value][key] = struct{}{}
	e.rangeIndex.Insert(value, key)
}

// removeFromIndexesLocked retire une clé des index. Appelée sous verrou par
// deleteLocked (et par setLocked avant réécriture d'une clé existante).
func (e *Engine) removeFromIndexesLocked(key, value string) {
	if set, ok := e.equalsIndex[value]; ok {
		delete(set, key)
		if len(set) == 0 {
			delete(e.equalsIndex, value)
		}
	}
	e.rangeIndex.Delete(value, key)
}

// GetWhere évalue un prédicat sur les valeurs du store et renvoie les
// entrées qui matchent, triées par clé pour un résultat déterministe.
func (e *Engine) GetWhere(op FilterOp, filterValue string) ([]Match, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch op {
	case OpEquals:
		return e.matchesFromKeySetLocked(e.equalsIndex[filterValue]), nil
	case OpContains:
		return e.scanContainsLocked(filterValue), nil
	case OpGT, OpGTE, OpLT, OpLTE:
		keys := e.rangeIndex.Range(op, filterValue)
		return e.matchesFromKeysLocked(keys), nil
	default:
		return nil, fmt.Errorf("opérateur non supporté: %s", op)
	}
}

// scanContainsLocked parcourt tout le state (pas d'index dédié pour une
// recherche de sous-chaîne) et filtre les valeurs contenant filterValue.
func (e *Engine) scanContainsLocked(filterValue string) []Match {
	matches := lo.FilterMap(lo.Entries(e.state), func(entry lo.Entry[string, string], _ int) (Match, bool) {
		return Match{Key: entry.Key, Value: entry.Value}, strings.Contains(entry.Value, filterValue)
	})
	sort.Slice(matches, func(i, j int) bool { return matches[i].Key < matches[j].Key })
	return matches
}

func (e *Engine) matchesFromKeySetLocked(keys map[string]struct{}) []Match {
	return e.matchesFromKeysLocked(lo.Keys(keys))
}

func (e *Engine) matchesFromKeysLocked(keys []string) []Match {
	matches := lo.Map(keys, func(key string, _ int) Match {
		return Match{Key: key, Value: e.state[key]}
	})
	sort.Slice(matches, func(i, j int) bool { return matches[i].Key < matches[j].Key })
	return matches
}

// sortKey normalise une valeur en une clé de tri lexicographique.
// Les nombres sont encodés sur une largeur fixe (via un décalage positif)
// pour que l'ordre lexicographique corresponde à l'ordre numérique
// (piège classique : "10" < "9" en tri de chaînes brutes).
// Les valeurs non numériques sont préfixées pour trier après tous les
// nombres, et comparées telles quelles entre elles.
func sortKey(value string) string {
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		const offset = 1e15 // large assez pour couvrir les valeurs attendues (âges, scores, prix...)
		return "0" + fmt.Sprintf("%030.6f", f+offset)
	}
	return "1" + value
}
