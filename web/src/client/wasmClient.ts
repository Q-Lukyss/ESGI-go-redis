import type {
  BatchResult,
  BrowseEntry,
  BrowseMode,
  BrowsePage,
  GoRedisClient,
  Match,
  PatchListener,
  Stats,
  StatsListener,
  Unsubscribe,
} from './types'
import { validateFilterOp, validateKey, validateTtl, validateValue } from './validate'

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
  matches?: Match[]
  results?: BatchResult[]
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

// buildWorkerConfigQuery traduit les variables Vite (VITE_*, cf.
// web/.env.example) en query string de l'URL du Worker — goredis-worker.js
// la retraduit en arguments CLI pour cmd/wasm/main.go (flag). Un Worker n'a
// pas de système de fichiers pour lire un .env directement : c'est le
// chemin le plus simple pour lui faire parvenir la config sans dupliquer
// les valeurs par défaut ailleurs qu'en Go (core.DefaultConfig) — un champ
// omis ici retombe simplement sur le défaut du flag côté Go.
function buildWorkerConfigQuery(): string {
  const env = import.meta.env
  const params = new URLSearchParams()
  const fields: [string, string | undefined][] = [
    ['aofFile', env.VITE_AOF_FILE],
    ['snapshotFile', env.VITE_SNAPSHOT_FILE],
    ['flushIntervalMs', env.VITE_FLUSH_INTERVAL_MS],
    ['snapshotIntervalMs', env.VITE_SNAPSHOT_INTERVAL_MS],
    ['expirySweepIntervalMs', env.VITE_EXPIRY_SWEEP_INTERVAL_MS],
    ['defaultTtlSeconds', env.VITE_DEFAULT_TTL_SECONDS],
    ['btreeDegree', env.VITE_BTREE_DEGREE],
    ['changesBufferSize', env.VITE_CHANGES_BUFFER_SIZE],
  ]
  for (const [key, value] of fields) {
    if (value !== undefined && value !== '') params.set(key, value)
  }
  return params.toString()
}

// createWasmClient démarre le Worker qui charge le moteur Go compilé en
// WASM (infrastructure/wasmbridge côté Go) et implémente GoRedisClient
// par-dessus un protocole request/response corrélé par requestId, puisque
// postMessage est par nature asynchrone et sans corrélation native.
export function createWasmClient(): WasmClientBundle {
  const query = buildWorkerConfigQuery()
  const worker = new Worker(query ? `/goredis-worker.js?${query}` : '/goredis-worker.js')
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
    async set(key, value, ttlSeconds) {
      validateKey(key)
      validateValue(value)
      validateTtl(ttlSeconds)
      const res = await request({ type: 'set', key, value, expireSeconds: ttlSeconds ?? 0 })
      if (res.error) throw new Error(res.error)
    },

    async get(key) {
      validateKey(key)
      const res = await request({ type: 'get', key })
      if (res.error) throw new Error(res.error)
      return res.value ?? ''
    },

    async delete(key) {
      validateKey(key)
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

    async query(op, value) {
      validateFilterOp(op)
      const res = await request({ type: 'query', op, filterValue: value })
      if (res.error) throw new Error(res.error)
      return res.matches ?? []
    },

    async batch(commands) {
      for (const c of commands) {
        validateKey(c.key)
        if (c.op === 'set') {
          validateValue(c.value ?? '')
          validateTtl(c.ttlSeconds)
        }
      }
      const res = await request({
        type: 'batch',
        commands: commands.map((c) => ({ op: c.op, key: c.key, value: c.value, expireSeconds: c.ttlSeconds ?? 0 })),
      })
      if (res.error) throw new Error(res.error)
      return res.results ?? []
    },

    async flushAll() {
      const res = await request({ type: 'flushall' })
      if (res.error) throw new Error(res.error)
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
