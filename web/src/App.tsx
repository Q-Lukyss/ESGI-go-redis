import { useEffect, useRef, useState } from 'react'
import { createServerClient } from './client/serverClient'
import type { BrowseMode, GoRedisClient, Stats } from './client/types'
import { StatsPanel } from './components/StatsPanel'
import { CrudPanel } from './components/CrudPanel'
import { InfiniteList } from './components/InfiniteList'
import './App.css'

export default function App() {
  // Initialisation paresseuse via ref (pas useState(() => ...)) : la
  // création du client ouvre une connexion WebSocket (effet de bord), et
  // StrictMode double-invoque les initialiseurs de useState en dev — ça
  // ouvrirait deux sockets. Le garde `if (!ref.current)` est le pattern
  // recommandé par React pour une construction paresseuse sûre sous
  // double-render.
  const clientRef = useRef<GoRedisClient | null>(null)
  if (!clientRef.current) {
    clientRef.current = createServerClient()
  }
  const client = clientRef.current

  const [mode, setMode] = useState<BrowseMode>('key')
  const [stats, setStats] = useState<Stats>({ stateCount: 0, bufferCount: 0 })

  useEffect(() => {
    client.getStats().then(setStats).catch(() => {})
    return client.onStats(setStats)
  }, [client])

  return (
    <div className="app">
      <h1>GoRedis — dashboard</h1>
      <StatsPanel stats={stats} />
      <CrudPanel client={client} />
      <div className="mode-toggle">
        <button className={mode === 'key' ? 'active' : ''} onClick={() => setMode('key')}>
          Browse (alphabétique)
        </button>
        <button className={mode === 'time' ? 'active' : ''} onClick={() => setMode('time')}>
          Activity (récent d'abord)
        </button>
      </div>
      {/* key={mode} : remonte la liste au changement de mode plutôt que de
          réconcilier son état interne (ordre + store) à la main. */}
      <InfiniteList key={mode} client={client} mode={mode} totalCount={stats.stateCount} />
    </div>
  )
}
