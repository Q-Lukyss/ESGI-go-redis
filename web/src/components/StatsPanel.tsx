import type { Stats } from '../client/types'

export function StatsPanel({ stats }: { stats: Stats }) {
  return (
    <div className="stats-panel">
      <div className="stat">
        <span className="stat-label">Clés (state)</span>
        <span className="stat-value">{stats.stateCount.toLocaleString('fr-FR')}</span>
      </div>
      <div className="stat">
        <span className="stat-label">Buffer AOF en attente</span>
        <span className="stat-value">{stats.bufferCount.toLocaleString('fr-FR')}</span>
      </div>
    </div>
  )
}
