package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"ESGI-go-redis/core"
)

func main() {
	fmt.Println("Démarage de GoRedis...")

	aofFile, snapshotFile, writeInterval, updateInterval := loadEnv()

	storage := core.NewFileStorage(aofFile, snapshotFile)
	goredis := core.NewGoRedis(storage, writeInterval, updateInterval)

	// ici on veut restaurer le state (snapshot + rejeu de l'AOF par-dessus)
	if err := goredis.Restore(); err != nil {
		fmt.Println("Erreur lors de la restauration du state :", err)
	}

	fmt.Println("GoRedis démarré.")
	printHelp()
	fmt.Println("En Attente de commandes : ")
	input := bufio.NewReader(os.Stdin)

	stop := make(chan struct{})
	defer close(stop)
	go goredis.RunFlushLoop(stop)
	go goredis.RunSnapshotLoop(stop)

	for {
		line, _ := input.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if handled := handleReplCommand(line); handled {
			continue
		}
		if strings.Contains(line, ";") {
			runBatch(goredis, line)
			continue
		}
		result, err := goredis.ExecuteString(line)
		printResult(result, err)
	}
}

func loadEnv() (string, string, time.Duration, time.Duration) {
	aofFile := "buffer_persistant.txt"
	snapshotFile := "state_persistant.json"
	writeInterval := 1 * time.Second
	updateInterval := 2 * time.Minute

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
	}
	return aofFile, snapshotFile, writeInterval, updateInterval
}

// handleReplCommand gère les commandes propres au REPL (pas au moteur) :
// quitter, afficher l'aide. Renvoie true si la ligne a été prise en charge ici.
func handleReplCommand(line string) bool {
	switch strings.ToUpper(line) {
	case "Q", "QUIT":
		fmt.Println("Arret du programme")
		os.Exit(0)
	case "H", "HELP":
		printHelp()
	default:
		return false
	}
	return true
}

// runBatch exécute plusieurs commandes séparées par ";" en un seul appel (Phase 3).
func runBatch(goredis *core.GoRedis, line string) {
	commands := strings.Split(line, ";")
	for i, cmd := range commands {
		commands[i] = strings.TrimSpace(cmd)
	}
	for _, result := range goredis.ExecuteBatch(commands) {
		printResult(result.Value, result.Err)
	}
}

func printHelp() {
	fmt.Println("Aide :")
	fmt.Println(`[SET <clé> "<valeur>"] : définit une clé avec une valeur`)
	fmt.Println("[DELETE <clé>] : supprime une clé")
	fmt.Println("[GET <clé>] : récupère la valeur d'une clé")
	fmt.Println(`[GET WHERE value equals "<valeur>"] : clés dont la valeur vaut exactement <valeur>`)
	fmt.Println(`[GET WHERE value contains "<sous-chaîne>"] : clés dont la valeur contient <sous-chaîne>`)
	fmt.Println("[GET WHERE value >|>=|<|<= <valeur>] : clés dont la valeur satisfait la comparaison")
	fmt.Println("[commande1 ; commande2 ; ...] : exécute plusieurs commandes en batch")
	fmt.Println("[q] : quitte le programme")
	fmt.Println("[h] : affiche l'aide")
}

func printResult(result any, err error) {
	if err != nil {
		fmt.Println("Erreur :", err)
		return
	}
	switch v := result.(type) {
	case nil:
		fmt.Println("OK")
	case []core.Match:
		if len(v) == 0 {
			fmt.Println("Aucun résultat")
			return
		}
		for _, match := range v {
			fmt.Printf("%s = %s\n", match.Key, match.Value)
		}
	default:
		fmt.Println(v)
	}
}
