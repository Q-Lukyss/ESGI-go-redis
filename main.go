package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"strings"

	"github.com/joho/godotenv"
	"github.com/samber/lo"
)

func main() {
	fmt.Println("Démarage de GoRedis...")

	// charger nos variables env
	stateFile, bufferFile, writeInterval, updateInterval := loadEnv()
	// pour eviter les erreurs
	fmt.Println(writeInterval)
	fmt.Println(updateInterval)

	goredis := NewGoRedis(bufferFile, stateFile, writeInterval, updateInterval)

	// ici on veut restaurer le state
	goredis.populate_state_on_startup()

	// on prompt l'user pour lui dire que tout est ok
	fmt.Println("GoRedis démarré.")
	fmt.Println("[q] Pour quitter")
	fmt.Println("[h] Pour afficher l'aide")
	fmt.Println("En Attente de commandes : ")
	input := bufio.NewReader(os.Stdin)

	// deux methodes pour mettre a jour le buffer et le state persistent
	// selon le tick
	go goredis.flush_memory_buffer()
	go goredis.save_persistent_state()

	// on veut une boucle infini qui run le tps du programme
	// elle prend les instructions
	for {
		line, _ := input.ReadString('\n')
		line = strings.TrimSpace(line)
		parseCommand(goredis.state, line)
		goredis.mutex.Lock()
		goredis.buffer = append(goredis.buffer, line)
		goredis.mutex.Unlock()
	}
}

func loadEnv() (string, string, time.Duration, time.Duration) {
	stateFile := "state_persistant.json"
	bufferFile := "buffer_persistant.txt"
	writeInterval := 1 * time.Second
	updateInterval := 2 * time.Minute

	if err := godotenv.Load(); err == nil {
		if v := os.Getenv("STATE_FILE"); v != "" {
			stateFile = v
		}
		if v := os.Getenv("BUFFER_FILE"); v != "" {
			bufferFile = v
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
	return stateFile, bufferFile, writeInterval, updateInterval
}

// | Commande | Forme | Effet |
// |----------|-------|-------|
// | `SET` | `SET <key> "<value>"` | `state[arg1] = arg2` |
// | `DELETE` | `DELETE <key>` | supprime la clé |
// | `GET` | `GET <key>` | renvoie la valeur |
// | `GET` filtré | `GET WHERE <champ> <op> <valeur>` | renvoie les entrées matchant le prédicat |

// Opérateurs de filtre à supporter sur `GET` : **`equals`, `contains`, `>`, `>=`, `<`, `<=`**.
func parseCommand(state map[string]any, input string) {
	args := strings.Fields(input)

	if len(args) == 0 {
		fmt.Println("Aucune commande saisie")
		return
	}

	switch args[0] {
	case "SET", "set", "Set":
		runSetCommand(state, args[1:])
	case "DELETE", "delete", "Delete":
		runDeleteCommand(state, args[1:])
	case "GET", "get", "Get":
		runGetCommand(state, args[1:])
	case "q", "Q", "quit", "Quit":
		fmt.Println("Arret du programme")
		os.Exit(0)
	case "h", "H", "help", "Help":
		fmt.Println("Aide :")
		fmt.Println("[SET] : définit une clé avec une valeur")
		fmt.Println("[DELETE] : supprime une clé")
		fmt.Println("[GET] : récupère la valeur d'une clé")
		fmt.Println("[q] : quitte le programme")
		fmt.Println("[h] : affiche l'aide")
	default:
		fmt.Println("Commande inconnue : " + args[0])
	}
}

func runSetCommand(state map[string]any, args []string) {
	fmt.Printf("Commande SET bien reçue : key : %s, value : %s\n", args[0], args[1])
	state[args[0]] = args[1]
}

func runDeleteCommand(state map[string]any, args []string) {
	fmt.Printf("Commande DELETE bien reçue : key : %s\n", args[0])
	delete(state, args[0])
}

func runGetCommand(state map[string]any, args []string) {
	fmt.Printf("Commande GET bien reçue : key : %s\n", args[0])
	// equivivalent de :
	// value, ok := state[args[0]]
	// if ok {
	if value, ok := state[args[0]]; ok { // equivalent de
		fmt.Println("value : pour ", args[0], " = ", value)
	} else {
		fmt.Println("Key non trouvée")
	}
}

func testLoLib() {
	names := []string{"alice", "bob", "charlie"}

	// Exemple simple
	upper := lo.Map(names, func(x string, _ int) string {
		return strings.ToUpper(x)
	})

	fmt.Println(upper)
}

type GoRedis struct {
	state                       map[string]any
	buffer                      []string
	bufferFile                  string
	stateFilePath               string
	writeToBufferInterval       time.Duration
	writePeristentStateInterval time.Duration
	mutex                       sync.Mutex
}

// Convention pour créer des structs en Go -> New + nom struct mdr
func NewGoRedis(bufferFile, stateFile string, writeInterval, updateInterval time.Duration) *GoRedis {
	return &GoRedis{
		state:                       make(map[string]any),
		buffer:                      []string{},
		bufferFile:                  bufferFile,
		stateFilePath:               stateFile,
		writeToBufferInterval:       writeInterval,
		writePeristentStateInterval: updateInterval,
		mutex:                       sync.Mutex{},
	}
}

func (g *GoRedis) flush_memory_buffer() {
	ticker := time.NewTicker(g.writeToBufferInterval)
	defer ticker.Stop()
	for range ticker.C {
		g.mutex.Lock()
		buffer_data := g.buffer
		g.buffer = nil
		g.mutex.Unlock()
		g.save_buffer_data_to_persistent_buffer(buffer_data)
	}
}

func (g *GoRedis) save_buffer_data_to_persistent_buffer(buffer_data []string) {
	if len(buffer_data) == 0 {
		return
	}
	file, err := os.OpenFile(g.bufferFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Erreur lors de l'ouverture du fichier buffer persistent:", err)
		return
	}
	defer file.Close()
	_, err = file.Write([]byte(strings.Join(buffer_data, "\n") + "\n"))
}

func (g *GoRedis) save_persistent_state() {
	// Ici on veut prendre le contenu du state
	// et construire un state au format json en ecrasant l'ancien j'imagine
	// La meilleure façon de faire c'est créer un fichier tmp
	// on ecrit dans le fichier tmp et ensuite on rename ce fichier pour ecraser l'ancien
	// ça evite la corruption des données car le rename est atomique
	// on pas d'ancien a nouveau nom sans etat intermediaire
	// Cependant mdr sur Windows evdemment
	// contrairement à Linux, os.Rename n'est pas atomique
	ticker := time.NewTicker(g.writePeristentStateInterval)
	defer ticker.Stop()
	for range ticker.C {
		g.mutex.Lock()
		// faire persister le state en json
		g.write_state_to_persistent_state(g.state)
		// vider le buffer persistant sous Lock
		// pour etre sur que on ecrit pas dedans
		os.Truncate(g.bufferFile, 0)
		g.mutex.Unlock()
	}
}

func (g *GoRedis) write_state_to_persistent_state(state_data map[string]any) {
	state_json, err := json.Marshal(state_data)
	if err != nil {
		fmt.Println("Erreur lors de la conversion du state en json : ", err)
		return
	}
	err = os.WriteFile("state.json.bak", state_json, 0644)
	if err != nil {
		fmt.Println("Erreur lors de l'écriture du fichier backup : ", err)
	}
	err = os.Rename("state.json.bak", "state.json")
	if err != nil {
		fmt.Println("Erreur lors du rename du fichier : ", err)
	}
}

func (g *GoRedis) populate_state_on_startup() {
	// Utilse lors du redémarage pour restaurer le state
	// on recupere le contenu json du state persistant json
	// on l'enrichie du contenu du buffer persistant
	// on restaure la map state à jour
	// content, err := os.ReadFile(g.bufferFile)
	// if err != nil {
	// 	fmt.Println("Erreur lors de la lecture du fichier : ", err)
	// }
	// for line := range strings.Split(string(content), "\n") {
	// 	fmt.Println(line)
	// }
	fmt.Println("TODO")
}
