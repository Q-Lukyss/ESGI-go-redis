package core

import "github.com/samber/lo"

// Result est le résultat d'une commande exécutée en batch : la valeur
// renvoyée (peut être nil, ex. pour un SET) et l'erreur éventuelle.
type Result struct {
	Value any
	Err   error
}

// ExecuteBatch exécute une liste de commandes déjà structurées (transport-
// agnostique : REST/WASM peuvent construire directement des Command sans
// passer par la syntaxe texte du REPL, cf. infrastructure/repl pour la
// variante qui parse des lignes brutes).
func (e *GoRedis) ExecuteBatch(cmds []Command) []Result {
	return lo.Map(cmds, func(cmd Command, _ int) Result {
		value, err := e.Execute(cmd)
		return Result{Value: value, Err: err}
	})
}
