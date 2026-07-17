// Package seed génère des jeux de données de démo, sans aucune I/O (ni
// fichier, ni réseau). Réutilisé tel quel par cmd/seed (natif, écrit sur
// disque via infrastructure/storage/file) et plus tard par le point
// d'entrée WASM, qui doit peupler l'OPFS depuis le navigateur — un
// process natif ne pouvant pas y écrire directement, la génération elle-
// même doit rester indépendante du support de stockage.
package seed

import (
	"fmt"
	"math/rand"
	"time"
)

// Entry est une paire clé/valeur horodatée, prête à être injectée via
// core.GoRedis.SetAt.
type Entry struct {
	Key       string
	Value     string
	Timestamp time.Time
}

const valueLength = 16

// Generate produit count entrées avec des timestamps répartis uniformément
// sur la fenêtre [now-spread, now], du plus ancien au plus récent — pour
// que la vue "Activity" (tri chronologique) ait une distribution réaliste
// plutôt que tout au même instant. randSeed rend le contenu des valeurs
// reproductible (benchmarks comparables d'un run à l'autre).
func Generate(count int, spread time.Duration, now time.Time, randSeed int64) []Entry {
	if count <= 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(randSeed))
	start := now.Add(-spread)
	step := spread
	if count > 1 {
		step = spread / time.Duration(count-1)
	}

	entries := make([]Entry, count)
	for i := 0; i < count; i++ {
		entries[i] = Entry{
			Key:       fmt.Sprintf("key:%08d", i),
			Value:     randomValue(rng),
			Timestamp: start.Add(time.Duration(i) * step),
		}
	}
	return entries
}

func randomValue(rng *rand.Rand) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, valueLength)
	for i := range b {
		b[i] = chars[rng.Intn(len(chars))]
	}
	return string(b)
}
