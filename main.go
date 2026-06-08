package main

import (
	"bufio"
	"fmt"
	"os"

	"strings"

	"github.com/samber/lo"
)

const STATE_FILE = "state_persistant.json"
const BUFFER_FILE_A = "buffer_persistant_a.txt"
const BUFFER_FILE_B = "buffer_persistant_b.txt"

func main() {
	fmt.Println("Démarage de GoRedis...")
	goredis := NewGoRedis()

	// ici on veut restaurer le state
	goredis.fullySynchronizeStateWithBuffer()

	// on prompt l'user pour lui dire que tout est ok
	fmt.Println("GoRedis démarré.")
	fmt.Println("[q] Pour quitter")
	fmt.Println("En Attente de commandes : ")
	input := bufio.NewReader(os.Stdin)

	// on veut une boucle infini qui run le tps du programme
	// elle prend les instructions
	for {
		line, _ := input.ReadString('\n')
		line = strings.TrimSpace(line)
		parseCommand(goredis.state, line) // pas besoin de passer un pointer car go le fait deja en interne
		goredis.buffer = append(goredis.buffer, line)
		// toute les secondes ensuite
		goredis.updateBuffer(&goredis.buffer)
		// toutes les 2 minutes ensuite
		goredis.updatePersistentState()
		fmt.Println("state : ", goredis.state)
		fmt.Println("buffer : ", goredis.buffer)
	}
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

func updateBuffer(buffer *[]string, mainBufferFile string) {
	os.WriteFile(mainBufferFile, []byte(strings.Join(*buffer, "\n")), 0644)
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
	state             map[string]any
	buffer            []string
	currentBufferFile string
	backupBufferFile  string
	stateFilePath     string
}

// Convention pour créer des structs en Go
func NewGoRedis() *GoRedis {
	return &GoRedis{
		state:             make(map[string]any),
		buffer:            []string{},
		currentBufferFile: BUFFER_FILE_A,
		backupBufferFile:  BUFFER_FILE_B,
		stateFilePath:     STATE_FILE,
	}
}

func (g *GoRedis) switchBufferFile() {
	if g.currentBufferFile == "buffer_persistant_a.txt" {
		g.currentBufferFile = "buffer_persistant_b.txt"
	} else {
		g.currentBufferFile = "buffer_persistant_a.txt"
	}
}

func (g *GoRedis) updateBuffer(buffer *[]string) {
	os.WriteFile(g.currentBufferFile, []byte(strings.Join(*buffer, "\n")), 0644)
}

func (g *GoRedis) updatePersistentState() {
	// Ici on veut prendre le contenu du buffer peristent
	// et reconstruire un state au format json
	return
}

func (g *GoRedis) fullySynchronizeStateWithBuffer() {
	// Utilse lors du redémarage pour restaurer le state
	// on recupere le contenu json du state persistant json
	// on l'enrichie du contenu du buffer persistant
	// on restaure la map state à jour
	content, err := os.ReadFile(g.currentBufferFile)
	if err != nil {
		fmt.Println("Erreur lors de la lecture du fichier : ", err)
	}
	for line := range strings.Split(string(content), "\n") {
		fmt.Println(line)
	}
}
