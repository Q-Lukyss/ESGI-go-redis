import { useEffect, useRef, useState } from 'react'
import { createServerClient } from './client/serverClient'
import { createWasmClient } from './client/wasmClient'
import type { BrowseMode, GoRedisClient, Stats } from './client/types'
import { StatsPanel } from './components/StatsPanel'
import { CrudPanel } from './components/CrudPanel'
import { InfiniteList } from './components/InfiniteList'
import { SeedPanel } from './components/SeedPanel'
import { BenchmarkPanel } from './components/BenchmarkPanel'
import './App.css'

type Backend = 'server' | 'wasm'

interface ClientBundle {
  client: GoRedisClient
  seed?: (count: number, spreadHours: number) => Promise<{ stateCount: number }>
}

function createBundle(backend: Backend): ClientBundle {
  if (backend === 'wasm') return createWasmClient()
  return { client: createServerClient() }
}

export default function App() {
  const [backend, setBackend] = useState<Backend>('server')
  const [mode, setMode] = useState<BrowseMode>('key')
  const [stats, setStats] = useState<Stats>({ stateCount: 0, bufferCount: 0 })

  // Le bundle doit être synchronisé avec `backend` DANS LE MÊME RENDU que
  // le clic qui le change — pas un rendu plus tard via useEffect. Sinon
  // <InfiniteList key={backend+mode}> se remonte (key changée) AVANT que
  // `client` ait suivi, avec son effet de premier chargement (dépendances
  // vides, ne se relance jamais) capturant l'ANCIEN client au passage :
  // bug réel rencontré ici (le panneau WASM affichait des données du
  // serveur). Le garde ci-dessous, exécuté pendant le rendu, tient les
  // deux toujours en phase ; StrictMode double-invoque le corps du
  // composant en dev, mais le garde le rend idempotent (la 2e invocation
  // voit déjà le bon bundle en ref et ne recrée rien).
  const bundleRef = useRef<{ backend: Backend; bundle: ClientBundle } | null>(null)
  if (!bundleRef.current || bundleRef.current.backend !== backend) {
    bundleRef.current?.bundle.client.dispose()
    bundleRef.current = { backend, bundle: createBundle(backend) }
  }
  const bundle = bundleRef.current.bundle

  useEffect(() => {
    setStats({ stateCount: 0, bufferCount: 0 })
    bundle.client.getStats().then(setStats).catch(() => {})
    return bundle.client.onStats(setStats)
  }, [bundle])

  return (
    <div className="app">
      <h1>GoRedis — dashboard</h1>

      <div className="backend-toggle">
        <button className={backend === 'server' ? 'active' : ''} onClick={() => setBackend('server')}>
          Backend serveur (REST + WS)
        </button>
        <button className={backend === 'wasm' ? 'active' : ''} onClick={() => setBackend('wasm')}>
          Backend WASM (Worker + OPFS)
        </button>
      </div>

      <StatsPanel stats={stats} />
      <CrudPanel client={bundle.client} />
      {bundle.seed && <SeedPanel seed={bundle.seed} />}
      <BenchmarkPanel client={bundle.client} backend={backend} />

      <div className="mode-toggle">
        <button className={mode === 'key' ? 'active' : ''} onClick={() => setMode('key')}>
          Browse (alphabétique)
        </button>
        <button className={mode === 'time' ? 'active' : ''} onClick={() => setMode('time')}>
          Activity (récent d'abord)
        </button>
      </div>
      {/* key={backend+mode} : remonte la liste au changement de backend ou
          de mode plutôt que de réconcilier son état interne à la main.
          Sûr désormais : bundle.client est toujours du bon backend dès ce
          même rendu (cf. garde ci-dessus). */}
      <InfiniteList key={`${backend}-${mode}`} client={bundle.client} mode={mode} totalCount={stats.stateCount} />
    </div>
  )
}
