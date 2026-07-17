import { useCallback, useEffect, useRef, useState } from 'react'
import type { BrowseMode, GoRedisClient } from '../client/types'
import { KeyedStore } from '../store/rowStore'
import { Row, type RowData } from './Row'

interface InfiniteListProps {
  client: GoRedisClient
  mode: BrowseMode
  totalCount: number // hauteur totale du spacer ; vient de /stats (approximatif tant que < chargé)
  rowHeight?: number
  viewportHeight?: number
  overscan?: number
}

const PAGE_SIZE = 100

// InfiniteList : virtualisation faite main (pas de react-window/virtuoso).
// Ne monte dans le DOM que les lignes visibles (+ overscan), positionnées
// en absolu ; un spacer porte la hauteur totale pour que la scrollbar
// reflète la vraie taille du store, même avant d'avoir tout chargé.
// L'ordre des clés (quelle ligne à quel index) vit dans `order`, séparé du
// contenu des lignes (dans `store`) : un patch de valeur ne touche que le
// store (rendu fin via Row/useRow), jamais `order`, donc jamais tout ce
// composant.
export function InfiniteList({
  client,
  mode,
  totalCount,
  rowHeight = 32,
  viewportHeight = 600,
  overscan = 6,
}: InfiniteListProps) {
  const storeRef = useRef(new KeyedStore<RowData>())
  const [order, setOrder] = useState<string[]>([])
  const cursorRef = useRef('')
  const [hasMore, setHasMore] = useState(true)
  const loadingRef = useRef(false)
  const [scrollTop, setScrollTop] = useState(0)

  const loadMore = useCallback(async () => {
    if (loadingRef.current || !hasMore) return
    loadingRef.current = true
    try {
      const page = await client.browse(mode, cursorRef.current, PAGE_SIZE)
      const store = storeRef.current
      const newKeys = page.entries.map((entry) => {
        store.set(entry.key, { value: entry.value, timestamp: entry.timestamp })
        return entry.key
      })
      setOrder((prev) => [...prev, ...newKeys])
      cursorRef.current = page.nextCursor
      setHasMore(page.hasMore)
    } finally {
      loadingRef.current = false
    }
  }, [client, mode, hasMore])

  // Premier chargement (et à chaque remount, cf. key={mode} côté App).
  useEffect(() => {
    void loadMore()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const startIndex = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan)
  const visibleCount = Math.ceil(viewportHeight / rowHeight) + overscan * 2
  const endIndex = Math.min(order.length, startIndex + visibleCount)

  // Charge la page suivante quand le scroll approche la fin du déjà-chargé.
  useEffect(() => {
    if (hasMore && !loadingRef.current && endIndex >= order.length - PAGE_SIZE / 2) {
      void loadMore()
    }
  }, [endIndex, order.length, hasMore, loadMore])

  // Déclare la fenêtre visible au client, pour que le serveur ne pousse
  // que les patchs pertinents (cf. infrastructure/ws.filterInWindow).
  useEffect(() => {
    const visible = order.slice(startIndex, endIndex)
    if (visible.length === 0) return
    if (mode === 'key') {
      client.setWindow('key', visible[0], visible[visible.length - 1])
    } else {
      const store = storeRef.current
      const first = store.get(visible[0])
      const last = store.get(visible[visible.length - 1])
      if (first && last) {
        client.setWindow('time', first.timestamp.toString(), last.timestamp.toString())
      }
    }
  }, [client, mode, order, startIndex, endIndex])

  // Applique les patchs entrants au store — ne touche jamais `order`,
  // donc ne re-render jamais la liste, seulement la ligne concernée.
  useEffect(() => {
    return client.onPatch((entries) => {
      const store = storeRef.current
      for (const entry of entries) {
        if (store.get(entry.key) !== undefined) {
          store.set(entry.key, { value: entry.value, timestamp: entry.timestamp })
        }
      }
    })
  }, [client])

  const visibleKeys = order.slice(startIndex, endIndex)
  const totalHeight = Math.max(totalCount, order.length) * rowHeight

  return (
    <div
      className="infinite-list"
      style={{ height: viewportHeight, overflowY: 'auto', position: 'relative' }}
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
    >
      <div style={{ height: totalHeight, position: 'relative' }}>
        {visibleKeys.map((key, i) => (
          <Row key={key} store={storeRef.current} rowKey={key} top={(startIndex + i) * rowHeight} height={rowHeight} />
        ))}
      </div>
    </div>
  )
}
