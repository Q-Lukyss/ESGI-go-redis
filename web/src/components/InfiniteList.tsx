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

// Taille de page du scroll infini (§7.2 : configurable, pas de constante en
// dur), surchargeable via VITE_PAGE_SIZE (web/.env, cf. .env.example).
const PAGE_SIZE = Number(import.meta.env.VITE_PAGE_SIZE) || 100

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
  // Plafonné par la taille totale connue (totalCount), pas par order.length :
  // sinon un saut de scroll au-delà du chargé donnerait endIndex < startIndex
  // et la fenêtre visible deviendrait vide au lieu d'afficher des
  // placeholders (cf. commentaire plus bas sur le rendu).
  const endIndex = Math.min(Math.max(totalCount, order.length), startIndex + visibleCount)

  // hasMore=false signifie "rien de plus à charger à la dernière tentative",
  // pas "le store ne grandira jamais" : avec le backend WASM, le store peut
  // être vide au montage (avant le seed déclenché depuis l'UI) puis se
  // remplir après coup. Sans ça, le premier browse() sur un store vide fixe
  // hasMore=false et plus rien ne relance jamais loadMore, même une fois le
  // seed terminé (bug réel rencontré ici : liste bloquée en placeholders).
  useEffect(() => {
    if (!hasMore && totalCount > order.length) {
      setHasMore(true)
    }
  }, [totalCount, order.length, hasMore])

  // Charge la page suivante quand le scroll approche la fin du déjà-chargé.
  useEffect(() => {
    if (hasMore && !loadingRef.current && endIndex >= order.length - PAGE_SIZE / 2) {
      void loadMore()
    }
  }, [endIndex, order.length, hasMore, loadMore])

  // Déclare la fenêtre visible au client, pour que le Worker ne pousse
  // que les patchs pertinents (cf. infrastructure/wasmbridge.filterInWindow).
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

  const totalHeight = Math.max(totalCount, order.length) * rowHeight

  // La pagination par curseur ne charge que séquentiellement depuis le
  // début (c'est le prix du O(log n) plutôt qu'un offset O(n) sur 1M
  // entrées) : un grand saut de scroll (glisser la scrollbar)
  // peut viser un index bien au-delà de ce qui est chargé. Plutôt que de
  // laisser un vide muet, on affiche un placeholder honnête pour tout index
  // pas encore chargé pendant que le chargement séquentiel rattrape.
  const visibleIndices = Array.from({ length: Math.max(0, endIndex - startIndex) }, (_, i) => startIndex + i)

  return (
    <div
      className="overflow-y-auto rounded-lg border border-slate-800 bg-slate-900/60 font-mono text-[13px]"
      style={{ height: viewportHeight, position: 'relative' }}
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
    >
      <div style={{ height: totalHeight, position: 'relative' }}>
        {visibleIndices.map((idx) => {
          const key = order[idx]
          if (key === undefined) {
            return (
              <div
                key={`loading-${idx}`}
                className="flex items-center gap-3 border-b border-slate-800/60 px-3 italic text-slate-500"
                style={{ position: 'absolute', top: idx * rowHeight, height: rowHeight, left: 0, right: 0 }}
              >
                …
              </div>
            )
          }
          return <Row key={key} store={storeRef.current} rowKey={key} top={idx * rowHeight} height={rowHeight} />
        })}
      </div>
    </div>
  )
}
