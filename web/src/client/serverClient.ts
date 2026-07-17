import type {
  BrowseEntry,
  BrowseMode,
  BrowsePage,
  GoRedisClient,
  PatchListener,
  Stats,
  StatsListener,
  Unsubscribe,
} from './types'

interface RawEntry {
  key: string
  value: string
  timestamp: string // json:",string" côté Go
}

function toBrowseEntry(raw: RawEntry): BrowseEntry {
  return { key: raw.key, value: raw.value, timestamp: BigInt(raw.timestamp) }
}

interface ServerClientOptions {
  baseUrl?: string
}

// createServerClient parle REST (CRUD/browse/stats) + WebSocket (patchs et
// stats en continu) à cmd/server. En dev, vite.config.ts proxy les chemins
// relatifs vers le serveur Go : pas besoin d'URL absolue ni de gérer CORS.
export function createServerClient(opts: ServerClientOptions = {}): GoRedisClient {
  const baseUrl = opts.baseUrl ?? ''

  const patchListeners = new Set<PatchListener>()
  const statsListeners = new Set<StatsListener>()

  let ws: WebSocket | null = null
  let currentWindow: { mode: BrowseMode; windowStart: string; windowEnd: string } | null = null
  let disposed = false

  function connect() {
    if (disposed) return
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = baseUrl ? baseUrl.replace(/^http/, 'ws') + '/ws' : `${proto}//${window.location.host}/ws`
    ws = new WebSocket(url)

    ws.addEventListener('open', () => {
      if (currentWindow) ws?.send(JSON.stringify(currentWindow))
    })

    ws.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data as string)
      if (msg.type === 'patch') {
        const entries = (msg.entries as RawEntry[]).map(toBrowseEntry)
        patchListeners.forEach((l) => l(entries))
      } else if (msg.type === 'stats') {
        const stats: Stats = { stateCount: msg.stateCount ?? 0, bufferCount: msg.bufferCount ?? 0 }
        statsListeners.forEach((l) => l(stats))
      }
    })

    // Reconnexion simple après une coupure : sans ça, un onglet resterait
    // silencieusement "mort" côté temps réel après le moindre hoquet réseau.
    // Sauf si dispose() a été appelé entre-temps (bascule de backend) :
    // sans ce garde, une fermeture volontaire relancerait quand même une
    // connexion fantôme.
    ws.addEventListener('close', () => {
      if (!disposed) setTimeout(connect, 1000)
    })
  }
  connect()

  async function request(path: string, init?: RequestInit): Promise<Response> {
    const res = await fetch(`${baseUrl}${path}`, init)
    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      throw new Error(body.error ?? `${res.status} ${res.statusText}`)
    }
    return res
  }

  return {
    async set(key, value) {
      await request(`/keys/${encodeURIComponent(key)}`, {
        method: 'PUT',
        body: JSON.stringify({ value }),
      })
    },

    async get(key) {
      const res = await request(`/keys/${encodeURIComponent(key)}`)
      const body = await res.json()
      return body.value
    },

    async delete(key) {
      await request(`/keys/${encodeURIComponent(key)}`, { method: 'DELETE' })
    },

    async browse(mode, cursor, limit): Promise<BrowsePage> {
      const params = new URLSearchParams({ mode, cursor, limit: String(limit) })
      const res = await request(`/browse?${params}`)
      const body = await res.json()
      return {
        entries: (body.entries as RawEntry[]).map(toBrowseEntry),
        nextCursor: body.nextCursor,
        hasMore: body.hasMore,
      }
    },

    async getStats() {
      const res = await request('/stats')
      return res.json()
    },

    onPatch(listener): Unsubscribe {
      patchListeners.add(listener)
      return () => patchListeners.delete(listener)
    },

    onStats(listener): Unsubscribe {
      statsListeners.add(listener)
      return () => statsListeners.delete(listener)
    },

    setWindow(mode, windowStart, windowEnd) {
      currentWindow = { mode, windowStart, windowEnd }
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(currentWindow))
      }
    },

    dispose() {
      disposed = true
      ws?.close()
    },
  }
}
