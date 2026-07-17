package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"ESGI-go-redis/core"
	httpapi "ESGI-go-redis/infrastructure/http"
	"ESGI-go-redis/infrastructure/repl"
	"ESGI-go-redis/infrastructure/storage/file"
	"ESGI-go-redis/infrastructure/ws"
)

func main() {
	fmt.Println("Démarage de GoRedis...")

	aofFile, snapshotFile, writeInterval, updateInterval, addr := loadEnv()

	storage := file.New(aofFile, snapshotFile)
	goredis := core.NewGoRedis(storage, writeInterval, updateInterval)

	// ici on veut restaurer le state (snapshot + rejeu de l'AOF par-dessus)
	if err := goredis.Restore(); err != nil {
		fmt.Println("Erreur lors de la restauration du state :", err)
	}

	fmt.Println("GoRedis démarré.")

	stop := make(chan struct{})
	defer close(stop)
	go goredis.RunFlushLoop(stop)
	go goredis.RunSnapshotLoop(stop)

	hub := ws.NewHub(goredis)
	go hub.Run(stop)

	router := httpapi.NewRouter(goredis, hub.Handler)
	go func() {
		fmt.Println("API REST + WS en écoute sur", addr)
		if err := http.ListenAndServe(addr, router); err != nil {
			log.Fatal("serveur HTTP arrêté :", err)
		}
	}()

	repl.Run(goredis)
}

func loadEnv() (aofFile, snapshotFile string, writeInterval, updateInterval time.Duration, addr string) {
	aofFile = "buffer_persistant.txt"
	snapshotFile = "state_persistant.json"
	writeInterval = 1 * time.Second
	updateInterval = 2 * time.Minute
	addr = ":8080"

	if err := godotenv.Load(); err == nil {
		if v := os.Getenv("BUFFER_FILE"); v != "" {
			aofFile = v
		}
		if v := os.Getenv("STATE_FILE"); v != "" {
			snapshotFile = v
		}
		if v := os.Getenv("WRITE_TO_BUFFER_INTERVAL"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				writeInterval = time.Duration(n) * time.Second
			}
		}
		if v := os.Getenv("UPDATE_STATE_FROM_BUFFER_INTERVAL"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				updateInterval = time.Duration(n) * time.Minute
			}
		}
		if v := os.Getenv("SERVER_ADDR"); v != "" {
			addr = v
		}
	}
	return aofFile, snapshotFile, writeInterval, updateInterval, addr
}
