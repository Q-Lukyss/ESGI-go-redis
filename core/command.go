package core

import (
	"fmt"
	"strings"
)

// CommandType identifie la nature d'une commande parsée.
type CommandType string

const (
	CmdSet      CommandType = "SET"
	CmdGet      CommandType = "GET"
	CmdDelete   CommandType = "DELETE"
	CmdGetWhere CommandType = "GET_WHERE"
)

// FilterOp est l'opérateur utilisé par un GET WHERE.
type FilterOp string

const (
	OpEquals   FilterOp = "equals"
	OpContains FilterOp = "contains"
	OpGT       FilterOp = ">"
	OpGTE      FilterOp = ">="
	OpLT       FilterOp = "<"
	OpLTE      FilterOp = "<="
)

// Command est la représentation structurée d'une ligne de commande.
// Selon Type, seuls certains champs sont pertinents :
//   - CmdSet    : Key, Value
//   - CmdGet    : Key
//   - CmdDelete : Key
//   - CmdGetWhere : FilterOp, FilterValue
type Command struct {
	Type        CommandType
	Key         string
	Value       string
	FilterOp    FilterOp
	FilterValue string
}

func ParseCommand(input string) (Command, error) {
	tokens := tokenize(input)
	if len(tokens) == 0 {
		return Command{}, fmt.Errorf("commande vide")
	}

	switch strings.ToUpper(tokens[0]) {
	case "SET":
		return parseSet(tokens)
	case "GET":
		return parseGet(tokens)
	case "DELETE":
		return parseDelete(tokens)
	default:
		return Command{}, fmt.Errorf("commande inconnue: %s", tokens[0])
	}
}

func parseSet(tokens []string) (Command, error) {
	if len(tokens) < 3 {
		return Command{}, fmt.Errorf("SET attend une clé et une valeur : SET <clé> \"<valeur>\"")
	}
	if len(tokens) > 3 {
		return Command{}, fmt.Errorf("SET n'accepte que 2 arguments (clé, valeur) : trop d'arguments")
	}
	return Command{Type: CmdSet, Key: tokens[1], Value: tokens[2]}, nil
}

func parseDelete(tokens []string) (Command, error) {
	if len(tokens) < 2 {
		return Command{}, fmt.Errorf("DELETE attend une clé : DELETE <clé>")
	}
	if len(tokens) > 2 {
		return Command{}, fmt.Errorf("DELETE n'accepte qu'un argument (clé) : trop d'arguments")
	}
	return Command{Type: CmdDelete, Key: tokens[1]}, nil
}

func parseGet(tokens []string) (Command, error) {
	if len(tokens) < 2 {
		return Command{}, fmt.Errorf("GET attend une clé : GET <clé>")
	}
	if strings.EqualFold(tokens[1], "WHERE") {
		return parseGetWhere(tokens)
	}
	if len(tokens) > 2 {
		return Command{}, fmt.Errorf("GET n'accepte qu'un argument (clé) : trop d'arguments")
	}
	return Command{Type: CmdGet, Key: tokens[1]}, nil
}

// parseGetWhere gère : GET WHERE <champ> <op> <valeur>
// Seul le champ "value" a un sens ici : le store est clé -> valeur (pas d'objets),
// donc filtrer "sur un champ" revient à filtrer sur la valeur elle-même.
func parseGetWhere(tokens []string) (Command, error) {
	if len(tokens) < 5 {
		return Command{}, fmt.Errorf("GET WHERE attend : GET WHERE <champ> <opérateur> <valeur>")
	}
	if len(tokens) > 5 {
		return Command{}, fmt.Errorf("GET WHERE : trop d'arguments")
	}
	field, opToken, filterValue := tokens[2], tokens[3], tokens[4]
	if !strings.EqualFold(field, "value") {
		return Command{}, fmt.Errorf("champ inconnu: %s (seul \"value\" est supporté)", field)
	}
	op, err := parseFilterOp(opToken)
	if err != nil {
		return Command{}, err
	}
	return Command{Type: CmdGetWhere, FilterOp: op, FilterValue: filterValue}, nil
}

func parseFilterOp(token string) (FilterOp, error) {
	switch strings.ToLower(token) {
	case string(OpEquals):
		return OpEquals, nil
	case string(OpContains):
		return OpContains, nil
	case string(OpGT):
		return OpGT, nil
	case string(OpGTE):
		return OpGTE, nil
	case string(OpLT):
		return OpLT, nil
	case string(OpLTE):
		return OpLTE, nil
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
