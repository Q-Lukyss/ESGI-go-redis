package repl

import (
	"fmt"
	"strings"

	"ESGI-go-redis/core"
)

// ParseCommand transforme une ligne texte (syntaxe REPL) en core.Command.
// C'est la seule pièce du moteur qui connaît cette syntaxe : REST/WASM
// construisent leurs Command directement, sans passer par ici.
func ParseCommand(input string) (core.Command, error) {
	tokens := tokenize(input)
	if len(tokens) == 0 {
		return core.Command{}, fmt.Errorf("commande vide")
	}

	switch strings.ToUpper(tokens[0]) {
	case "SET":
		return parseSet(tokens)
	case "GET":
		return parseGet(tokens)
	case "DELETE":
		return parseDelete(tokens)
	default:
		return core.Command{}, fmt.Errorf("commande inconnue: %s", tokens[0])
	}
}

func parseSet(tokens []string) (core.Command, error) {
	if len(tokens) < 3 {
		return core.Command{}, fmt.Errorf("SET attend une clé et une valeur : SET <clé> \"<valeur>\"")
	}
	if len(tokens) > 3 {
		return core.Command{}, fmt.Errorf("SET n'accepte que 2 arguments (clé, valeur) : trop d'arguments")
	}
	return core.Command{Type: core.CmdSet, Key: tokens[1], Value: tokens[2]}, nil
}

func parseDelete(tokens []string) (core.Command, error) {
	if len(tokens) < 2 {
		return core.Command{}, fmt.Errorf("DELETE attend une clé : DELETE <clé>")
	}
	if len(tokens) > 2 {
		return core.Command{}, fmt.Errorf("DELETE n'accepte qu'un argument (clé) : trop d'arguments")
	}
	return core.Command{Type: core.CmdDelete, Key: tokens[1]}, nil
}

func parseGet(tokens []string) (core.Command, error) {
	if len(tokens) < 2 {
		return core.Command{}, fmt.Errorf("GET attend une clé : GET <clé>")
	}
	if strings.EqualFold(tokens[1], "WHERE") {
		return parseGetWhere(tokens)
	}
	if len(tokens) > 2 {
		return core.Command{}, fmt.Errorf("GET n'accepte qu'un argument (clé) : trop d'arguments")
	}
	return core.Command{Type: core.CmdGet, Key: tokens[1]}, nil
}

// parseGetWhere gère : GET WHERE <champ> <op> <valeur>
// Seul le champ "value" a un sens ici : le store est clé -> valeur (pas d'objets),
// donc filtrer "sur un champ" revient à filtrer sur la valeur elle-même.
func parseGetWhere(tokens []string) (core.Command, error) {
	if len(tokens) < 5 {
		return core.Command{}, fmt.Errorf("GET WHERE attend : GET WHERE <champ> <opérateur> <valeur>")
	}
	if len(tokens) > 5 {
		return core.Command{}, fmt.Errorf("GET WHERE : trop d'arguments")
	}
	field, opToken, filterValue := tokens[2], tokens[3], tokens[4]
	if !strings.EqualFold(field, "value") {
		return core.Command{}, fmt.Errorf("champ inconnu: %s (seul \"value\" est supporté)", field)
	}
	op, err := parseFilterOp(opToken)
	if err != nil {
		return core.Command{}, err
	}
	return core.Command{Type: core.CmdGetWhere, FilterOp: op, FilterValue: filterValue}, nil
}

func parseFilterOp(token string) (core.FilterOp, error) {
	switch strings.ToLower(token) {
	case string(core.OpEquals):
		return core.OpEquals, nil
	case string(core.OpContains):
		return core.OpContains, nil
	case string(core.OpGT):
		return core.OpGT, nil
	case string(core.OpGTE):
		return core.OpGTE, nil
	case string(core.OpLT):
		return core.OpLT, nil
	case string(core.OpLTE):
		return core.OpLTE, nil
	default:
		return "", fmt.Errorf("opérateur inconnu: %s (attendu: equals, contains, >, >=, <, <=)", token)
	}
}

// tokenize découpe une ligne en tokens séparés par des espaces, en respectant
// les guillemets doubles : SET msg "hello world" -> ["SET", "msg", "hello world"].
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false
	hasCurrent := false

	flush := func() {
		if hasCurrent {
			tokens = append(tokens, current.String())
			current.Reset()
			hasCurrent = false
		}
	}

	for _, r := range input {
		switch r {
		case '"':
			inQuotes = !inQuotes
			hasCurrent = true
		case ' ', '\t':
			if inQuotes {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
			hasCurrent = true
		}
	}
	flush()

	return tokens
}
