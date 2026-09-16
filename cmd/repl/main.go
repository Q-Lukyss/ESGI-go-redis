package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"ESGI-go-redis/core"
	"ESGI-go-redis/infrastructure/repl"
	"ESGI-go-redis/infrastructure/storage/file"
)

func main() {
	fmt.Println("Démarage de GoRedis...")

	aofFile, snapshotFile, cfg := loadEnv()

	storage := file.New(aofFile, snapshotFile)
	goredis := core.NewGoRedis(storage, cfg)

	// ici on veut restaurer le state (snapshot + rejeu de l'AOF par-dessus)
	if err := goredis.Restore(); err != nil {
		fmt.Println("Erreur lors de la restauration du state :", err)
	}

	fmt.Println("GoRedis démarré.")

	stop := make(chan struct{})
	defer close(stop)
	go goredis.RunFlushLoop(stop)
	go goredis.RunSnapshotLoop(stop)
	go goredis.RunExpirySweepLoop(stop)

	repl.Run(goredis)
}

// loadEnv lit .env (si présent) et retombe sur core.DefaultConfig() pour
// tout ce qui n'est pas surchargé : aucune valeur de tuning n'est câblée en
// dur ici, seulement des noms de variables (cf. .env.example).
func loadEnv() (aofFile, snapshotFile string, cfg core.Config) {
	aofFile = "buffer_persistant.txt"
	snapshotFile = "state_persistant.json"
	cfg = core.DefaultConfig()

	if err := godotenv.Load(); err == nil {
		if v := os.Getenv("BUFFER_FILE"); v != "" {
			aofFile = v
		}
		if v := os.Getenv("STATE_FILE"); v != "" {
			snapshotFile = v
		}
		if v := os.Getenv("WRITE_TO_BUFFER_INTERVAL"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.FlushInterval = time.Duration(n) * time.Second
			}
		}
		if v := os.Getenv("UPDATE_STATE_FROM_BUFFER_INTERVAL"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.SnapshotInterval = time.Duration(n) * time.Minute
			}
		}
		if v := os.Getenv("EXPIRY_SWEEP_INTERVAL"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.ExpirySweepInterval = time.Duration(n) * time.Second
			}
		}
		if v := os.Getenv("DEFAULT_TTL_SECONDS"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				cfg.DefaultTTLSeconds = n
			}
		}
		if v := os.Getenv("BTREE_DEGREE"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 2 {
				cfg.BTreeDegree = n
			}
		}
		if v := os.Getenv("CHANGES_BUFFER_SIZE"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.ChangesBufferSize = n
			}
		}
	}
	return aofFile, snapshotFile, cfg
}
