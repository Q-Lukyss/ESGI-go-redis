import { useRef, useState } from 'react'
import { initWasmRedis, type WasmRedis } from '../sdk/initWasmRedis'

// Schema du SDK pour cette démo : le typage par générique de initWasmRedis
// vérifie à la compile que seules ces clés sont utilisables avec set/get/
// delete/cmd.* — ex. `db.get('nonexistent')` ci-dessous ne compilerait pas
// si décommenté (clé absente de Schema).
type Schema = { name: string; age: string; note: string }

// SdkDemoPanel exécute, contre le vrai Worker+WASM (pas un mock), la
// séquence exacte demandée par le cahier des charges §5.1 : set direct,
// batch (set/delete/set-avec-EX en un seul aller-retour), puis un query
// builder chaîné (.where().where().exec()) — la conjonction de plusieurs
// filtres est résolue côté SDK (intersection des clés), le moteur Go
// n'acceptant qu'un seul prédicat par requête GET WHERE.
//
// Note d'intégration : initWasmRedis() démarre son PROPRE Worker (comme le
// veut le cahier), distinct de celui du dashboard. cmd/wasm ne permet pas
// encore de choisir un autre
// nom de fichier OPFS au runtime (aof.log/snapshot.json sont en dur) : les
// deux instances de moteur partagent alors le même stockage. Sans risque de
// corruption (le sync access handle OPFS est exclusif par nature), mais les
// deux copies en RAM peuvent diverger l'une de l'autre pendant la session.
// Acceptable pour cette démo isolée ; dans une vraie appli n'embarquant que
// le SDK (le cas d'usage réel visé par le cahier), il n'y a qu'un seul
// Worker et la question ne se pose pas.
export function SdkDemoPanel() {
  const dbRef = useRef<WasmRedis<Schema> | null>(null)
  const [log, setLog] = useState<string[]>([])
  const [running, setRunning] = useState(false)

  function append(line: string) {
    setLog((prev) => [...prev, line])
  }

  async function handleRun() {
    setRunning(true)
    setLog([])
    try {
      const db = dbRef.current ?? (dbRef.current = await initWasmRedis<Schema>())

      await db.set('name', 'matt')
      await db.set('age', '30')
      append(`db.get('name') = "${await db.get('name')}"`)

      // Erreur typée : clé absente. Ne pas confondre avec une erreur hors
      // Schema (celle-là est détectée à la compile, pas au runtime).
      await db.delete('note').catch(() => append("db.delete('note') -> GoRedisKeyNotFoundError (clé déjà absente)"))

      await db.batch([db.cmd.set('note', 'hello'), db.cmd.set('age', '31', { ex: 60 }), db.cmd.delete('name')])
      append("db.batch([set note, set age EX 60s, delete name]) -> OK (1 aller-retour Worker)")

      const matches = await db.get().where('equals', 'hello').where('contains', 'hell').exec()
      append(`db.get().where('equals','hello').where('contains','hell').exec() -> ${JSON.stringify(matches)}`)

      // Ne compile pas si décommenté : "unknown" n'est pas une clé de Schema.
      // await db.get('unknown')
    } catch (err) {
      append(`Erreur : ${(err as Error).message}`)
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-4">
      <button
        type="button"
        onClick={handleRun}
        disabled={running}
        className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500 disabled:opacity-50"
      >
        {running ? 'En cours…' : 'Exécuter la démo SDK (initWasmRedis<Schema>)'}
      </button>
      {log.length > 0 && (
        <pre className="mt-3 whitespace-pre-wrap rounded-md bg-slate-950 p-3 font-mono text-xs text-slate-300">{log.join('\n')}</pre>
      )}
    </div>
  )
}
