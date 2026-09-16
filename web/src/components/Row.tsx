import { memo } from 'react'
import { KeyedStore, useRow, bumpRenderCount } from '../store/rowStore'

export interface RowData {
  value: string
  timestamp: bigint
}

interface RowProps {
  store: KeyedStore<RowData>
  rowKey: string
  top: number
  height: number
}

function RowImpl({ store, rowKey, top, height }: RowProps) {
  const data = useRow(store, rowKey)
  const renders = bumpRenderCount(rowKey)

  return (
    <div
      className="flex items-center gap-3 border-b border-slate-800/60 px-3 text-slate-300"
      style={{ position: 'absolute', top, height, left: 0, right: 0 }}
    >
      <span className="w-36 shrink-0 overflow-hidden text-ellipsis whitespace-nowrap text-slate-200">{rowKey}</span>
      <span className="flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{data?.value ?? '…'}</span>
      <span className="text-[11px] text-slate-600" title="nombre de rendus de cette ligne">
        ×{renders}
      </span>
    </div>
  )
}

// memo : ceinture-bretelles. Le vrai mécanisme de rendu fin est le
// useSyncExternalStore par clé ci-dessus (une ligne non concernée par un
// changement n'est jamais notifiée) ; memo évite en plus un re-render si le
// parent (InfiniteList) re-render pour une raison qui ne change pas `top`.
export const Row = memo(RowImpl)
