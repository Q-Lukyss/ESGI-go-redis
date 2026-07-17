// GoRedisClient est l'abstraction commune aux deux backends possibles
// (serveur REST+WS aujourd'hui, WASM+Worker plus tard). Les composants UI
// ne dépendent que de cette interface, jamais du transport : basculer d'un
// backend à l'autre ne touche à aucun composant.

export type BrowseMode = 'key' | 'time'

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
  set(key: string, value: string): Promise<void>
  get(key: string): Promise<string>
  delete(key: string): Promise<void>
  browse(mode: BrowseMode, cursor: string, limit: number): Promise<BrowsePage>
  getStats(): Promise<Stats>

  // Mises à jour de valeur sur des clés déjà chargées (patch en place).
  onPatch(listener: PatchListener): Unsubscribe
  // Compteurs globaux, en continu.
  onStats(listener: StatsListener): Unsubscribe
  // Déclare la fenêtre actuellement visible, pour que le transport ne
  // pousse que les patchs pertinents (cf. infrastructure/ws côté Go).
  setWindow(mode: BrowseMode, windowStart: string, windowEnd: string): void

  // Ferme la connexion sous-jacente (WebSocket ou Worker). À appeler avant
  // de recréer un client (bascule de backend) ou au démontage de l'app.
  dispose(): void
}
