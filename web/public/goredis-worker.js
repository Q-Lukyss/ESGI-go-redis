// Charge et exécute le moteur GoRedis compilé en WASM. Tourne
// obligatoirement dans un Worker dédié : c'est la seule contexte où l'OPFS
// expose des sync access handles (cf. infrastructure/storage/opfs). Go
// prend la main dès go.run() : c'est lui qui installe self.onmessage
// (infrastructure/wasmbridge.Bridge.Start) et pousse ses messages via
// self.postMessage — ce script n'a rien d'autre à faire.
importScripts('/wasm_exec.js')

const go = new Go()
// Un Worker n'a pas de système de fichiers pour lire un .env comme
// cmd/server : la config (§7.2) transite donc par la query string de l'URL
// du Worker (cf. wasmClient.ts, qui la construit depuis import.meta.env),
// traduite en arguments CLI (flag) que cmd/wasm/main.go parse au démarrage.
const params = new URLSearchParams(self.location.search)
go.argv = ['js', ...Array.from(params.entries(), ([key, value]) => `-${key}=${value}`)]

WebAssembly.instantiateStreaming(fetch('/goredis.wasm'), go.importObject).then((result) => {
  go.run(result.instance)
})
