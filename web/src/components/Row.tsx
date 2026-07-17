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
    <div className="row" style={{ position: 'absolute', top, height, left: 0, right: 0 }}>
      <span className="row-key">{rowKey}</span>
      <span className="row-value">{data?.value ?? '…'}</span>
      <span className="row-renders" title="nombre de rendus de cette ligne">
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
