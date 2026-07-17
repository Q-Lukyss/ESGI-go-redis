//go:build js && wasm

// cmd/wasm est le point d'entrée du backend WASM : moteur core.GoRedis +
// stockage OPFS + pont wasmbridge, sans REPL (n'a pas de sens dans un
// Worker navigateur). Construit avec :
//
//	GOOS=js GOARCH=wasm go build -o web/public/goredis.wasm ./cmd/wasm
//
// Chargé et exécuté par web/public/goredis-worker.js, dans un Worker dédié
// (contrainte OPFS : les sync access handles n'existent que là).
package main

import (
	"fmt"
	"time"

	"ESGI-go-redis/core"
	"ESGI-go-redis/infrastructure/storage/opfs"
	"ESGI-go-redis/infrastructure/wasmbridge"
)

func main() {
	storage := opfs.New("aof.log", "snapshot.json")
	engine := core.NewGoRedis(storage, time.Second, 2*time.Minute)

	// stdout/stderr Go sont câblés sur la console du navigateur par le
	// runtime WASM : fmt.Println suffit comme diagnostic depuis le Worker.
	if err := engine.Restore(); err != nil {
		fmt.Println("Erreur lors de la restauration du state :", err)
	}

	stop := make(chan struct{})
	go engine.RunFlushLoop(stop)
	go engine.RunSnapshotLoop(stop)

	wasmbridge.New(engine).Start(stop) // bloque : c'est la boucle de vie du Worker
}
