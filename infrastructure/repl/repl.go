// Package repl est l'adaptateur CLI : boucle interactive stdin, parsing de
// la syntaxe texte ("SET clé \"valeur\"") et exécution contre un
// core.GoRedis. C'est le seul endroit qui connaît cette syntaxe — REST et
// wasmbridge construisent leurs core.Command directement.
package repl

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/samber/lo"

	"ESGI-go-redis/core"
)

// Run démarre la boucle interactive stdin : lit des lignes, gère les
// commandes REPL (q/h), le batch texte ("cmd1;cmd2"), et l'exécution
// simple, jusqu'à interruption (Ctrl+C / EOF).
func Run(e *core.GoRedis) {
	PrintHelp()
	fmt.Println("En Attente de commandes : ")
	input := bufio.NewReader(os.Stdin)

	for {
		line, _ := input.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if handleReplCommand(line) {
			continue
		}
		if strings.Contains(line, ";") {
			runBatch(e, line)
			continue
		}
		result, err := ExecuteLine(e, line)
		PrintResult(result, err)
	}
}

// ExecuteLine parse une ligne brute puis l'exécute, en propageant l'erreur de parsing.
func ExecuteLine(e *core.GoRedis, line string) (any, error) {
	cmd, err := ParseCommand(line)
	if err != nil {
		return nil, err
	}
	return e.Execute(cmd)
}

// ExecuteBatchLines parse et exécute plusieurs lignes texte ; une ligne
// invalide ne bloque pas les autres (résultat aligné, un Result par ligne).
func ExecuteBatchLines(e *core.GoRedis, lines []string) []core.Result {
	return lo.Map(lines, func(line string, _ int) core.Result {
		value, err := ExecuteLine(e, line)
		return core.Result{Value: value, Err: err}
	})
}

// handleReplCommand gère les commandes propres au REPL (pas au moteur) :
// quitter, afficher l'aide. Renvoie true si la ligne a été prise en charge ici.
func handleReplCommand(line string) bool {
	switch strings.ToUpper(line) {
	case "Q", "QUIT":
		fmt.Println("Arret du programme")
		os.Exit(0)
	case "H", "HELP":
		PrintHelp()
	default:
		return false
	}
	return true
}

// runBatch exécute plusieurs commandes séparées par ";" en un seul appel (Phase 3).
func runBatch(e *core.GoRedis, line string) {
	lines := strings.Split(line, ";")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	for _, result := range ExecuteBatchLines(e, lines) {
		PrintResult(result.Value, result.Err)
	}
}

func PrintHelp() {
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

func PrintResult(result any, err error) {
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
