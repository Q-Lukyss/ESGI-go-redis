// GoRedisClient est l'abstraction commune aux deux backends possibles
// (serveur REST+WS aujourd'hui, WASM+Worker plus tard). Les composants UI
// ne dépendent que de cette interface, jamais du transport : basculer d'un
// backend à l'autre ne touche à aucun composant.

export type BrowseMode = 'key' | 'time'

// FilterOp reprend les 6 opérateurs de GET WHERE côté moteur (core.FilterOp).
export type FilterOp = 'equals' | 'contains' | '>' | '>=' | '<' | '<='

export interface Match {
  key: string
  value: string
}

// BatchCommand est la forme cliente d'une commande de batch : set/get/delete
// seulement (cf. cahier des charges §3.6), ttlSeconds optionnel sur set.
export interface BatchCommand {
  op: 'set' | 'get' | 'delete'
  key: string
  value?: string
  ttlSeconds?: number
}

export interface BatchResult {
  value?: string
  error?: string
}

export interface BrowseEntry {
  key: string
  value: string
  // unix-nano : au-delà de 2^53 un `number` JS perdrait en précision, d'où
  // bigint (le serveur encode ce champ en chaîne exprès pour ça).
  timestamp: bigint
}

export interface BrowsePage {
  entries: BrowseEntry[]
  nextCursor: string
  hasMore: boolean
}

export interface Stats {
  stateCount: number
  bufferCount: number
}

export type PatchListener = (entries: BrowseEntry[]) => void
export type StatsListener = (stats: Stats) => void
export type Unsubscribe = () => void

export interface GoRedisClient {
  // ttlSeconds : expiration en secondes (TTL). Absent ou <= 0 = pas de TTL,
  // la clé reste persistante (comportement Redis standard).
  set(key: string, value: string, ttlSeconds?: number): Promise<void>
  get(key: string): Promise<string>
  delete(key: string): Promise<void>
  browse(mode: BrowseMode, cursor: string, limit: number): Promise<BrowsePage>
  getStats(): Promise<Stats>
  // GET WHERE : filtre sur la valeur (equals/contains résolus par index,
  // les 4 opérateurs de range résolus par le B-Tree côté moteur).
  query(op: FilterOp, value: string): Promise<Match[]>
  // Plusieurs commandes en un seul aller-retour transport (§3.6).
  batch(commands: BatchCommand[]): Promise<BatchResult[]>
  // Supprime toutes les clés (FLUSHALL) et persiste immédiatement l'état vide.
  flushAll(): Promise<void>

  // Mises à jour de valeur sur des clés déjà chargées (patch en place).
  onPatch(listener: PatchListener): Unsubscribe
  // Compteurs globaux, en continu.
  onStats(listener: StatsListener): Unsubscribe
  // Déclare la fenêtre actuellement visible, pour que le transport ne
  // pousse que les patchs pertinents (cf. infrastructure/wasmbridge/window.go).
  setWindow(mode: BrowseMode, windowStart: string, windowEnd: string): void

  // Ferme la connexion sous-jacente (WebSocket ou Worker). À appeler avant
  // de recréer un client (bascule de backend) ou au démontage de l'app.
  dispose(): void
}
