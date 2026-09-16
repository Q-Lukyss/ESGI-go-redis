// cmd/seed peuple le stockage natif (AOF + snapshot) avec un jeu de
// données de démo, avant de démarrer cmd/repl. Un process natif ne peut
// pas écrire dans l'OPFS d'un navigateur : la variante WASM devra peupler
// depuis le navigateur lui-même, en réutilisant seed.Generate.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"

	"ESGI-go-redis/core"
	"ESGI-go-redis/infrastructure/storage/file"
	"ESGI-go-redis/seed"
)

func main() {
	count := flag.Int("count", 1_000_000, "nombre de clés à générer")
	spread := flag.Duration("spread", 720*time.Hour, "fenêtre temporelle sur laquelle répartir les timestamps (ex: 720h = 30 jours)")
	randSeed := flag.Int64("seed", 42, "graine du générateur pseudo-aléatoire (reproductibilité)")
	flag.Parse()

	aofFile, snapshotFile := loadEnv()
	storage := file.New(aofFile, snapshotFile)
	// TTL et balayage n'ont pas de sens ici : aucune goroutine de fond n'est
	// lancée (pas de RunFlushLoop/RunExpirySweepLoop), le process fait son
	// travail puis s'arrête via Snapshot() explicite ci-dessous.
	cfg := core.DefaultConfig()
	cfg.ExpirySweepInterval = 0
	cfg.DefaultTTLSeconds = 0
	engine := core.NewGoRedis(storage, cfg)

	fmt.Printf("Génération de %d clés (fenêtre %s)...\n", *count, *spread)
	genStart := time.Now()
	entries := seed.Generate(*count, *spread, genStart, *randSeed)
	fmt.Printf("Génération terminée en %s. Peuplement du moteur...\n", time.Since(genStart))

	loadStart := time.Now()
	for _, e := range entries {
		engine.SetAt(e.Key, e.Value, e.Timestamp)
	}
	fmt.Printf("Peuplement terminé en %s (%d clés en state).\n", time.Since(loadStart), engine.StateCount())

	snapshotStart := time.Now()
	if err := engine.Snapshot(); err != nil {
		fmt.Println("Erreur lors du snapshot :", err)
		os.Exit(1)
	}
	fmt.Printf("Snapshot écrit en %s -> %s\n", time.Since(snapshotStart), snapshotFile)
}

func loadEnv() (aofFile, snapshotFile string) {
	aofFile = "buffer_persistant.txt"
	snapshotFile = "state_persistant.json"
	if err := godotenv.Load(); err == nil {
		if v := os.Getenv("BUFFER_FILE"); v != "" {
			aofFile = v
		}
		if v := os.Getenv("STATE_FILE"); v != "" {
			snapshotFile = v
		}
	}
	return aofFile, snapshotFile
}
