//go:build js && wasm

// cmd/wasm est le point d'entrée du backend WASM : moteur core.GoRedis +
// stockage OPFS + pont wasmbridge, sans REPL (n'a pas de sens dans un
// Worker navigateur). Construit avec :
//
//	GOOS=js GOARCH=wasm go build -o web/public/goredis.wasm ./cmd/wasm
//
// Chargé et exécuté par web/public/goredis-worker.js, dans un Worker dédié
// (contrainte OPFS : les sync access handles n'existent que là). Un Worker
// n'a pas de système de fichiers pour lire un .env comme cmd/repl : la
// configuration (§7.2, aucune constante en dur) est donc passée en
// arguments CLI (flag), que goredis-worker.js construit à partir des
// variables Vite (VITE_*, cf. web/.env.example) au moment de créer le
// Worker — mêmes noms de constantes, deux sources de config différentes.
package main

import (
	"flag"
	"fmt"
	"time"

	"ESGI-go-redis/core"
	"ESGI-go-redis/infrastructure/storage/opfs"
	"ESGI-go-redis/infrastructure/wasmbridge"
)

func main() {
	cfg := loadConfig()

	storage := opfs.New(*flagAofFile, *flagSnapshotFile)
	engine := core.NewGoRedis(storage, cfg)

	// stdout/stderr Go sont câblés sur la console du navigateur par le
	// runtime WASM : fmt.Println suffit comme diagnostic depuis le Worker.
	if err := engine.Restore(); err != nil {
		fmt.Println("Erreur lors de la restauration du state :", err)
	}

	stop := make(chan struct{})
	go engine.RunFlushLoop(stop)
	go engine.RunSnapshotLoop(stop)
	go engine.RunExpirySweepLoop(stop)

	wasmbridge.New(engine).Start(stop) // bloque : c'est la boucle de vie du Worker
}

var (
	flagAofFile      *string
	flagSnapshotFile *string
)

// loadConfig déclare les flags CLI (un par champ de core.Config, plus les
// noms de fichiers OPFS), les valeurs par défaut étant celles de
// core.DefaultConfig() — puis parse os.Args (peuplé par goredis-worker.js
// via go.argv avant go.run()).
func loadConfig() core.Config {
	def := core.DefaultConfig()

	flagAofFile = flag.String("aofFile", "aof.log", "nom du fichier AOF dans l'OPFS")
	flagSnapshotFile = flag.String("snapshotFile", "snapshot.json", "nom du fichier snapshot dans l'OPFS")
	flushMs := flag.Int64("flushIntervalMs", def.FlushInterval.Milliseconds(), "cycle de flush du buffer vers l'AOF, en ms")
	snapshotMs := flag.Int64("snapshotIntervalMs", def.SnapshotInterval.Milliseconds(), "cycle de snapshot + compaction, en ms")
	sweepMs := flag.Int64("expirySweepIntervalMs", def.ExpirySweepInterval.Milliseconds(), "fréquence du balayage des clés expirées, en ms (0 = désactivé)")
	defaultTTL := flag.Int64("defaultTtlSeconds", def.DefaultTTLSeconds, "TTL appliqué à un SET sans EX explicite, en secondes (0 = désactivé)")
	btreeDegree := flag.Int("btreeDegree", def.BTreeDegree, "ordre du B-Tree (keyIndex/timeIndex/rangeIndex)")
	changesBufferSize := flag.Int("changesBufferSize", def.ChangesBufferSize, "taille du channel de notification interne")
	flag.Parse()

	return core.Config{
		FlushInterval:       time.Duration(*flushMs) * time.Millisecond,
		SnapshotInterval:    time.Duration(*snapshotMs) * time.Millisecond,
		ExpirySweepInterval: time.Duration(*sweepMs) * time.Millisecond,
		DefaultTTLSeconds:   *defaultTTL,
		BTreeDegree:         *btreeDegree,
		ChangesBufferSize:   *changesBufferSize,
	}
}
