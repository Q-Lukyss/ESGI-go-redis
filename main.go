package main

import (
	"bufio"
	"fmt"
	"os"

	"strings"

	"github.com/samber/lo"
)

func main() {
	fmt.Println("Démarage de GoRedis...")
	// Init state et buffer
	var state map[string]any
	var buffer []string

	fmt.Println("GoRedis démarré.")
	fmt.Println("[q] Pour quitter")
	fmt.Println("En Attente de commandes : ")
	// on veut une boucle infini qui run le tps du programme
	// elle prend les instructions

	for {
		input := bufio.NewReader(os.Stdin)
		line, _ := input.ReadString('\n')
		line = strings.TrimSpace(line)
		parseCommand(&state, line)
		buffer = append(buffer, line)
		updateBuffer(&buffer)
	}
}

// | Commande | Forme | Effet |
// |----------|-------|-------|
// | `SET` | `SET <key> "<value>"` | `state[arg1] = arg2` |
// | `DELETE` | `DELETE <key>` | supprime la clé |
// | `GET` | `GET <key>` | renvoie la valeur |
// | `GET` filtré | `GET WHERE <champ> <op> <valeur>` | renvoie les entrées matchant le prédicat |

// Opérateurs de filtre à supporter sur `GET` : **`equals`, `contains`, `>`, `>=`, `<`, `<=`**.
func parseCommand(state *map[string]any, input string) {
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

func runSetCommand(state *map[string]any, args []string) {
	fmt.Printf("Commande SET bien reçue : key : %s, value : %s\n", args[0], args[1])
}

func runDeleteCommand(state *map[string]any, args []string) {
	fmt.Printf("Commande DELETE bien reçue : key : %s\n", args[0])
}

func runGetCommand(state *map[string]any, args []string) {
	fmt.Printf("Commande GET bien reçue : key : %s\n", args[0])
}

func updateBuffer(buffer *[]string) {
	os.WriteFile("buffer_persistant.txt", []byte(strings.Join(*buffer, "\n")), 0644)
}

func testLoLib() {
	names := []string{"alice", "bob", "charlie"}

	// Exemple simple
	upper := lo.Map(names, func(x string, _ int) string {
		return strings.ToUpper(x)
	})

	fmt.Println(upper)
}
