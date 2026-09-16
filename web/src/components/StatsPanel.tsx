import type { Stats } from '../client/types'

export function StatsPanel({ stats }: { stats: Stats }) {
  return (
    <div className="flex gap-4">
      <div className="flex min-w-40 flex-col gap-1 rounded-lg border border-slate-800 bg-slate-900/60 px-4 py-3">
        <span className="text-xs text-slate-400">Clés (state)</span>
        <span className="font-mono text-xl tabular-nums text-slate-100">{stats.stateCount.toLocaleString('fr-FR')}</span>
      </div>
      <div className="flex min-w-40 flex-col gap-1 rounded-lg border border-slate-800 bg-slate-900/60 px-4 py-3">
        <span className="text-xs text-slate-400">Buffer AOF en attente</span>
        <span className="font-mono text-xl tabular-nums text-slate-100">{stats.bufferCount.toLocaleString('fr-FR')}</span>
      </div>
    </div>
  )
}
