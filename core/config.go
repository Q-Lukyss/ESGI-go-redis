package core

import "time"

// Config regroupe toutes les constantes de tuning du moteur, pour respecter
// §7.2 (aucune constante codée en dur) : un unique point d'entrée avec des
// valeurs par défaut (DefaultConfig), surchargeables par l'appelant selon sa
// propre source de configuration — cmd/repl lit un .env (fichier), cmd/wasm
// lit ses arguments CLI transmis par le Worker JS, lui-même informé par les
// variables Vite (VITE_*) au build (cf. web/public/goredis-worker.js et
// web/.env.example pour la chaîne complète jusqu'au moteur Go/WASM).
type Config struct {
	FlushInterval       time.Duration // cycle de vidage du buffer d'écritures vers l'AOF
	SnapshotInterval    time.Duration // cycle de snapshot + compaction de l'AOF
	ExpirySweepInterval time.Duration // fréquence du balayage des clés expirées ; <= 0 désactive le balayage
	DefaultTTLSeconds   int64         // TTL appliqué à un SET sans EX explicite ; <= 0 désactive
	BTreeDegree         int           // ordre du B-Tree (keyIndex/timeIndex/rangeIndex)
	ChangesBufferSize   int           // taille du channel de notification (GoRedis.Changes)
}

// DefaultConfig donne les valeurs par défaut du moteur. Ni cmd/repl ni
// cmd/wasm ne redéfinissent ces nombres eux-mêmes : ils lisent leur propre
// source de config (fichier .env ou arguments CLI) et ne retombent sur ces
// valeurs qu'en l'absence de surcharge.
func DefaultConfig() Config {
	return Config{
		FlushInterval:       time.Second,
		SnapshotInterval:    2 * time.Minute,
		ExpirySweepInterval: 10 * time.Second,
		DefaultTTLSeconds:   0,
		BTreeDegree:         3,
		ChangesBufferSize:   4096,
	}
}
