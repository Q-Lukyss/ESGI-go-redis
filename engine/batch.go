package engine

import "github.com/samber/lo"

// Result est le résultat d'une commande exécutée en batch : la valeur
// renvoyée (peut être nil, ex. pour un SET) et l'erreur éventuelle.
type Result struct {
	Value any
	Err   error
}

// ExecuteBatch exécute une liste de commandes texte dans l'ordre et renvoie
// la liste des résultats, alignée sur l'ordre des commandes en entrée.
func (e *Engine) ExecuteBatch(lines []string) []Result {
	return lo.Map(lines, func(line string, _ int) Result {
		value, err := e.ExecuteString(line)
		return Result{Value: value, Err: err}
	})
}
