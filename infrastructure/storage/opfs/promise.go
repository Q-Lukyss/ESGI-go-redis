//go:build js && wasm

package opfs

import (
	"errors"
	"syscall/js"
)

// awaitPromise bloque la goroutine appelante jusqu'à résolution d'une
// Promise JS, sans bloquer la boucle d'évènements du navigateur : le
// scheduler Go/WASM rend la main tant que le channel n'a rien reçu, et la
// goroutine reprend quand le callback .then/.catch écrit dessus. C'est le
// pont standard entre l'API OPFS (getDirectory/getFileHandle/
// createSyncAccessHandle, toutes asynchrones) et l'interface core.Storage
// (synchrone par conception).
func awaitPromise(promise js.Value) (js.Value, error) {
	type outcome struct {
		value js.Value
		err   error
	}
	ch := make(chan outcome, 1)

	onFulfilled := js.FuncOf(func(this js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- outcome{value: v}
		return nil
	})
	defer onFulfilled.Release()

	onRejected := js.FuncOf(func(this js.Value, args []js.Value) any {
		msg := "promesse rejetée"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		ch <- outcome{err: errors.New(msg)}
		return nil
	})
	defer onRejected.Release()

	promise.Call("then", onFulfilled, onRejected)

	o := <-ch
	return o.value, o.err
}
