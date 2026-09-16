import { useState, type FormEvent } from 'react'
import type { FilterOp, GoRedisClient, Match } from '../client/types'

const OPS: FilterOp[] = ['equals', 'contains', '>', '>=', '<', '<=']

const inputClass =
  'rounded-md border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-sm text-slate-200 placeholder:text-slate-500 focus:border-violet-500 focus:outline-none'

// QueryPanel démontre GET WHERE (§3.1) : equals/contains résolus par scan
// ou index inversé, les 4 opérateurs de range résolus par le B-Tree côté
// moteur (core.RangeIndex) — jamais par un scan complet.
export function QueryPanel({ client }: { client: GoRedisClient }) {
  const [op, setOp] = useState<FilterOp>('equals')
  const [value, setValue] = useState('')
  const [matches, setMatches] = useState<Match[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      setMatches(await client.query(op, value))
    } catch (err) {
      setError((err as Error).message)
      setMatches(null)
    }
  }

  return (
    <form
      className="flex flex-wrap items-center gap-2 rounded-lg border border-slate-800 bg-slate-900/60 p-4"
      onSubmit={handleSubmit}
    >
      <span className="text-sm text-slate-400">GET WHERE value</span>
      <select value={op} onChange={(e) => setOp(e.target.value as FilterOp)} className={inputClass}>
        {OPS.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
      <input className={inputClass} placeholder="valeur" value={value} onChange={(e) => setValue(e.target.value)} />
      <button type="submit" className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500">
        Chercher
      </button>
      {error && <span className="text-xs text-red-400">Erreur : {error}</span>}
      {matches && (
        <span className="text-xs text-slate-400">
          {matches.length === 0 ? 'Aucun résultat' : `${matches.length} résultat(s) : ${matches
            .slice(0, 10)
            .map((m) => `${m.key}=${m.value}`)
            .join(', ')}${matches.length > 10 ? '…' : ''}`}
        </span>
      )}
    </form>
  )
}
