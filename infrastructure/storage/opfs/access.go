//go:build js && wasm

package opfs

import "syscall/js"

// getSyncAccessHandle ouvre (en la créant si besoin) filename à la racine
// de l'OPFS et renvoie un FileSystemSyncAccessHandle. N'est disponible que
// dans un Worker dédié — c'est une contrainte de la spec OPFS, pas de ce
// code, d'où l'obligation que cmd/wasm tourne dans un Worker.
// L'appelant est responsable de handle.Call("close") une fois fini :
// createSyncAccessHandle prend un verrou exclusif sur le fichier tant que
// le handle n'est pas fermé.
func getSyncAccessHandle(filename string) (js.Value, error) {
	storage := js.Global().Get("navigator").Get("storage")

	dir, err := awaitPromise(storage.Call("getDirectory"))
	if err != nil {
		return js.Value{}, err
	}

	fileHandle, err := awaitPromise(dir.Call("getFileHandle", filename, map[string]any{"create": true}))
	if err != nil {
		return js.Value{}, err
	}

	syncHandle, err := awaitPromise(fileHandle.Call("createSyncAccessHandle"))
	if err != nil {
		return js.Value{}, err
	}
	return syncHandle, nil
}

// readAll lit tout le contenu du fichier derrière handle. Les méthodes de
// FileSystemSyncAccessHandle (read/write/getSize/truncate/flush) sont
// synchrones une fois le handle obtenu — contrairement à son acquisition,
// pas besoin d'awaitPromise ici.
func readAll(handle js.Value) []byte {
	size := handle.Call("getSize").Int()
	if size == 0 {
		return nil
	}
	jsBuf := js.Global().Get("Uint8Array").New(size)
	n := handle.Call("read", jsBuf, map[string]any{"at": 0}).Int()
	buf := make([]byte, n)
	js.CopyBytesToGo(buf, jsBuf)
	return buf
}

// writeAll remplace tout le contenu du fichier par data.
func writeAll(handle js.Value, data []byte) {
	handle.Call("truncate", 0)
	if len(data) == 0 {
		handle.Call("flush")
		return
	}
	jsBuf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsBuf, data)
	handle.Call("write", jsBuf, map[string]any{"at": 0})
	handle.Call("flush")
}

// appendBytes ajoute data à la fin du fichier, sans toucher au contenu existant.
func appendBytes(handle js.Value, data []byte) {
	if len(data) == 0 {
		return
	}
	offset := handle.Call("getSize").Int()
	jsBuf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsBuf, data)
	handle.Call("write", jsBuf, map[string]any{"at": offset})
	handle.Call("flush")
}
