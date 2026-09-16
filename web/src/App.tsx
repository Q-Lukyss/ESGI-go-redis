import { useEffect, useRef, useState } from 'react'
import { createWasmClient, type WasmClientBundle } from './client/wasmClient'
import type { BrowseMode, Stats } from './client/types'
import { StatsPanel } from './components/StatsPanel'
import { CrudPanel } from './components/CrudPanel'
import { BatchPanel } from './components/BatchPanel'
import { QueryPanel } from './components/QueryPanel'
import { InfiniteList } from './components/InfiniteList'
import { SeedPanel } from './components/SeedPanel'
import { BenchmarkPanel } from './components/BenchmarkPanel'
import { SdkDemoPanel } from './components/SdkDemoPanel'

export default function App() {
  const [mode, setMode] = useState<BrowseMode>('key')
  const [stats, setStats] = useState<Stats>({ stateCount: 0, bufferCount: 0 })
  // Incrémenté après un flushAll pour forcer le remontage d'InfiniteList
  // (son état de pagination interne n'a plus de sens sur un store vidé),
  // même pattern que le remontage sur changement de `mode`.
  const [resetToken, setResetToken] = useState(0)

  // Singleton : un seul Worker WASM pour toute la durée de vie de l'app,
  // recréé uniquement si StrictMode double-invoque le corps du composant en
  // dev (la ref le rend idempotent). Pas de dispose au démontage : App est
  // la racine, montée une seule fois pour toute la session — un effet de
  // nettoyage ici serait lui-même invoqué deux fois par StrictMode en dev
  // (mount -> unmount fantôme -> remount), ce qui terminait le Worker réel
  // dès sa création sans jamais le recréer (le garde ci-dessus ne s'exécute
  // qu'en phase de rendu, pas quand les effets se rejouent). Le navigateur
  // ferme le Worker de lui-même à la fermeture/navigation de l'onglet.
  const bundleRef = useRef<WasmClientBundle | null>(null)
  if (!bundleRef.current) {
    bundleRef.current = createWasmClient()
  }
  const bundle = bundleRef.current

  useEffect(() => {
    bundle.client.getStats().then(setStats).catch(() => {})
    return bundle.client.onStats(setStats)
  }, [bundle])

  async function handleFlushAll() {
    if (!window.confirm('Supprimer toutes les clés ? Cette action est irréversible.')) return
    await bundle.client.flushAll()
    setStats(await bundle.client.getStats())
    setResetToken((t) => t + 1)
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-8 text-slate-200">
      <header className="mb-8 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-slate-50">GoRedis — dashboard</h1>
          <p className="mt-1 text-sm text-slate-400">Moteur Go compilé en WASM, exécuté dans un Web Worker, persistance OPFS.</p>
        </div>
        <button
          type="button"
          onClick={handleFlushAll}
          className="shrink-0 rounded-md bg-red-600/80 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-500"
        >
          Tout supprimer
        </button>
      </header>

      <div className="flex flex-col gap-6">
        <StatsPanel stats={stats} />
        <CrudPanel client={bundle.client} />
        <BatchPanel client={bundle.client} />
        <QueryPanel client={bundle.client} />
        <SeedPanel seed={bundle.seed} />
        <BenchmarkPanel client={bundle.client} />
        <SdkDemoPanel />

        <div>
          <div className="mb-3 flex gap-2">
            <button
              type="button"
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${mode === 'key' ? 'bg-violet-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
              onClick={() => setMode('key')}
            >
              Browse (alphabétique)
            </button>
            <button
              type="button"
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${mode === 'time' ? 'bg-violet-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
              onClick={() => setMode('time')}
            >
              Activity (récent d'abord)
            </button>
          </div>
          {/* key={mode+resetToken} : remonte la liste au changement de mode ou
              après un flushAll, plutôt que de réconcilier son état interne à
              la main. */}
          <InfiniteList key={`${mode}-${resetToken}`} client={bundle.client} mode={mode} totalCount={stats.stateCount} />
        </div>
      </div>
    </div>
  )
}
