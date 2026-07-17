import { useSyncExternalStore } from 'react'

type Listener = () => void

// KeyedStore : chaque ligne s'abonne individuellement à sa propre clé. Une
// mise à jour ne notifie que les abonnés de CETTE clé — les autres lignes
// ne sont même jamais informées qu'un changement a eu lieu. C'est ce qui
// garantit le rendu fin (une seule ligne re-render), contrairement à
// React.memo seul qui ne fait que déduire l'égalité de props après-coup :
// ici il n'y a structurellement rien à comparer pour les lignes non
// concernées.
export class KeyedStore<T> {
  private data = new Map<string, T>()
  private listeners = new Map<string, Set<Listener>>()

  get(key: string): T | undefined {
    return this.data.get(key)
  }

  set(key: string, value: T): void {
    this.data.set(key, value)
    this.emit(key)
  }

  delete(key: string): void {
    this.data.delete(key)
    this.emit(key)
  }

  subscribe(key: string, listener: Listener): () => void {
    let set = this.listeners.get(key)
    if (!set) {
      set = new Set()
      this.listeners.set(key, set)
    }
    set.add(listener)
    return () => {
      set!.delete(listener)
      if (set!.size === 0) this.listeners.delete(key)
    }
  }

  private emit(key: string): void {
    this.listeners.get(key)?.forEach((listener) => listener())
  }
}

export function useRow<T>(store: KeyedStore<T>, key: string): T | undefined {
  return useSyncExternalStore(
    (onStoreChange) => store.subscribe(key, onStoreChange),
    () => store.get(key),
  )
}

// renderCounts : compteur de rendu par ligne, exposé sur window pour la
// démonstration du rendu fin (React DevTools "highlight updates" marche
// aussi, mais un compteur explicite est plus facile à vérifier en continu).
export const renderCounts = new Map<string, number>()

export function bumpRenderCount(key: string): number {
  const next = (renderCounts.get(key) ?? 0) + 1
  renderCounts.set(key, next)
  return next
}
