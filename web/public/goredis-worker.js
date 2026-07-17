// Charge et exécute le moteur GoRedis compilé en WASM. Tourne
// obligatoirement dans un Worker dédié : c'est la seule contexte où l'OPFS
// expose des sync access handles (cf. infrastructure/storage/opfs). Go
// prend la main dès go.run() : c'est lui qui installe self.onmessage
// (infrastructure/wasmbridge.Bridge.Start) et pousse ses messages via
// self.postMessage — ce script n'a rien d'autre à faire.
importScripts('/wasm_exec.js')

const go = new Go()
WebAssembly.instantiateStreaming(fetch('/goredis.wasm'), go.importObject).then((result) => {
  go.run(result.instance)
})
