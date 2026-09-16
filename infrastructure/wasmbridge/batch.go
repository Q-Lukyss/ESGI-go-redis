//go:build js && wasm

package wasmbridge

import (
	"fmt"

	"github.com/samber/lo"

	"ESGI-go-redis/core"
)

// toCoreCommand traduit une commande de batch reçue du fil (set/get/delete
// seulement, cf. cahier des charges §3.6) en core.Command, transport-agnostique.
func toCoreCommand(bc batchCommand) (core.Command, error) {
	if err := validateKey(bc.Key); err != nil {
		return core.Command{}, err
	}
	switch bc.Op {
	case "set":
		if err := validateValue(bc.Value); err != nil {
			return core.Command{}, err
		}
		if err := validateExpireSeconds(bc.ExpireSeconds); err != nil {
			return core.Command{}, err
		}
		return core.Command{Type: core.CmdSet, Key: bc.Key, Value: bc.Value, ExpireSeconds: bc.ExpireSeconds}, nil
	case "get":
		return core.Command{Type: core.CmdGet, Key: bc.Key}, nil
	case "delete":
		return core.Command{Type: core.CmdDelete, Key: bc.Key}, nil
	default:
		return core.Command{}, fmt.Errorf("opération de batch inconnue: %s (attendu: set, get, delete)", bc.Op)
	}
}

// toCoreCommands convertit tout le lot ; une seule commande invalide fait
// échouer le batch entier avant exécution (pas d'exécution partielle sur un
// lot malformé — cf. validation à la frontière, §7.4).
func toCoreCommands(batch []batchCommand) ([]core.Command, error) {
	cmds := make([]core.Command, len(batch))
	for i, bc := range batch {
		cmd, err := toCoreCommand(bc)
		if err != nil {
			return nil, err
		}
		cmds[i] = cmd
	}
	return cmds, nil
}

// toBatchResults aplatit []core.Result (Value any) en []batchResult
// sérialisable : seul CmdGet renvoie une valeur exploitable (string), les
// autres n'ont qu'un statut ok/erreur.
func toBatchResults(results []core.Result) []batchResult {
	return lo.Map(results, func(r core.Result, _ int) batchResult {
		if r.Err != nil {
			return batchResult{Error: r.Err.Error()}
		}
		if s, ok := r.Value.(string); ok {
			return batchResult{Value: s}
		}
		return batchResult{}
	})
}
