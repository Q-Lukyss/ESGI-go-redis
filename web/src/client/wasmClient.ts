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
  timestamp: string
}

function toBrowseEntry(raw: RawEntry): BrowseEntry {
  return { key: raw.key, value: raw.value, timestamp: BigInt(raw.timestamp) }
}

interface WorkerMessage {
  type: string
  requestId?: string
  value?: string
  entries?: RawEntry[]
  nextCursor?: string
  hasMore?: boolean
  stateCount?: number
  bufferCount?: number
  error?: string
}

let requestCounter = 0
function nextRequestId(): string {
  requestCounter += 1
  return `wasm-${requestCounter}`
}

export interface WasmClientBundle {
  client: GoRedisClient
  // Peuplement de démo déclenché depuis le navigateur : un process natif
  // ne peut pas écrire dans l'OPFS d'un onglet (cf. infrastructure/
  // wasmbridge). Propre au backend WASM, donc hors de GoRedisClient (le
  // serveur natif se peuple via cmd/seed, pas depuis l'UI).
  seed(count: number, spreadHours: number): Promise<{ stateCount: number }>
}

// createWasmClient démarre le Worker qui charge le moteur Go compilé en
// WASM (infrastructure/wasmbridge côté Go) et implémente GoRedisClient
// par-dessus un protocole request/response corrélé par requestId, puisque
// postMessage est par nature asynchrone et sans corrélation native.
export function createWasmClient(): WasmClientBundle {
  const worker = new Worker('/goredis-worker.js')
  const patchListeners = new Set<PatchListener>()
  const statsListeners = new Set<StatsListener>()
  const pending = new Map<string, (msg: WorkerMessage) => void>()
  let currentWindow: { mode: BrowseMode; windowStart: string; windowEnd: string } | null = null
  let disposed = false

  let resolveReady: () => void
  const ready = new Promise<void>((res) => {
    resolveReady = res
  })

  worker.onmessage = (ev: MessageEvent<WorkerMessage>) => {
    const msg = ev.data
    if (msg.type === 'ready') {
      resolveReady()
      if (currentWindow) worker.postMessage({ type: 'subscribe', ...currentWindow })
      return
    }
    if ((msg.type === 'response' || msg.type === 'seedDone') && msg.requestId && pending.has(msg.requestId)) {
      pending.get(msg.requestId)!(msg)
      pending.delete(msg.requestId)
      return
    }
    if (msg.type === 'patch') {
      const entries = (msg.entries ?? []).map(toBrowseEntry)
      patchListeners.forEach((l) => l(entries))
    } else if (msg.type === 'stats') {
      const stats: Stats = { stateCount: msg.stateCount ?? 0, bufferCount: msg.bufferCount ?? 0 }
      statsListeners.forEach((l) => l(stats))
    }
  }

  async function request(payload: Record<string, unknown>): Promise<WorkerMessage> {
    await ready
    const requestId = nextRequestId()
    return new Promise((resolve) => {
      pending.set(requestId, resolve)
      worker.postMessage({ ...payload, requestId })
    })
  }

  const client: GoRedisClient = {
    async set(key, value) {
      const res = await request({ type: 'set', key, value })
      if (res.error) throw new Error(res.error)
    },

    async get(key) {
      const res = await request({ type: 'get', key })
      if (res.error) throw new Error(res.error)
      return res.value ?? ''
    },

    async delete(key) {
      const res = await request({ type: 'delete', key })
      if (res.error) throw new Error(res.error)
    },

    async browse(mode, cursor, limit): Promise<BrowsePage> {
      const res = await request({ type: 'browse', mode, cursor, limit })
      if (res.error) throw new Error(res.error)
      return {
        entries: (res.entries ?? []).map(toBrowseEntry),
        nextCursor: res.nextCursor ?? '',
        hasMore: res.hasMore ?? false,
      }
    },

    async getStats() {
      const res = await request({ type: 'stats' })
      return { stateCount: res.stateCount ?? 0, bufferCount: res.bufferCount ?? 0 }
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
      ready.then(() => {
        if (!disposed) worker.postMessage({ type: 'subscribe', mode, windowStart, windowEnd })
      })
    },

    dispose() {
      disposed = true
      worker.terminate()
    },
  }

  return {
    client,
    async seed(count, spreadHours) {
      const res = await request({ type: 'seed', count, spreadHours })
      if (res.error) throw new Error(res.error)
      return { stateCount: res.stateCount ?? 0 }
    },
  }
}
