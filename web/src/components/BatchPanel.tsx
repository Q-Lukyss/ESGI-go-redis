import { useState } from 'react'
import type { BatchCommand, BatchResult, GoRedisClient } from '../client/types'

type Row = { op: 'set' | 'get' | 'delete'; key: string; value: string; ttl: string }

const emptyRow = (): Row => ({ op: 'set', key: '', value: '', ttl: '' })

const inputClass =
  'rounded-md border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-sm text-slate-200 placeholder:text-slate-500 focus:border-violet-500 focus:outline-none'

// BatchPanel compose plusieurs commandes (set/get/delete) et les envoie en un
// seul appel client.batch() — un seul aller-retour Worker (cf. cahier des
// charges §3.6), résultats alignés sur les lignes envoyées.
export function BatchPanel({ client }: { client: GoRedisClient }) {
  const [rows, setRows] = useState<Row[]>([emptyRow()])
  const [results, setResults] = useState<BatchResult[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  function updateRow(i: number, patch: Partial<Row>) {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))
  }

  function addRow() {
    setRows((prev) => [...prev, emptyRow()])
  }

  function removeRow(i: number) {
    setRows((prev) => prev.filter((_, idx) => idx !== i))
  }

  async function handleSubmit() {
    setError(null)
    setRunning(true)
    try {
      const commands: BatchCommand[] = rows
        .filter((r) => r.key.trim() !== '')
        .map((r) => ({
          op: r.op,
          key: r.key,
          value: r.op === 'set' ? r.value : undefined,
          ttlSeconds: r.op === 'set' && r.ttl ? Number(r.ttl) : undefined,
        }))
      setResults(await client.batch(commands))
    } catch (err) {
      setError((err as Error).message)
      setResults(null)
    } finally {
      setRunning(false)
    }
  }

  return (
    <section className="rounded-lg border border-slate-800 bg-slate-900/60 p-4">
      <h2 className="mb-3 text-sm font-semibold text-slate-300">Batch (plusieurs commandes, 1 aller-retour)</h2>
      <div className="flex flex-col gap-2">
        {rows.map((row, i) => (
          <div key={i} className="flex flex-wrap items-center gap-2">
            <select value={row.op} onChange={(e) => updateRow(i, { op: e.target.value as Row['op'] })} className={inputClass}>
              <option value="set">SET</option>
              <option value="get">GET</option>
              <option value="delete">DELETE</option>
            </select>
            <input className={inputClass} placeholder="clé" value={row.key} onChange={(e) => updateRow(i, { key: e.target.value })} />
            {row.op === 'set' && (
              <>
                <input
                  className={inputClass}
                  placeholder="valeur"
                  value={row.value}
                  onChange={(e) => updateRow(i, { value: e.target.value })}
                />
                <input
                  className={`${inputClass} w-28`}
                  type="number"
                  min="1"
                  placeholder="TTL (s)"
                  value={row.ttl}
                  onChange={(e) => updateRow(i, { ttl: e.target.value })}
                />
              </>
            )}
            <button
              type="button"
              onClick={() => removeRow(i)}
              disabled={rows.length === 1}
              className="rounded-md bg-slate-800 px-2.5 py-1.5 text-sm text-slate-300 transition-colors hover:bg-slate-700 disabled:opacity-40"
            >
              ✕
            </button>
          </div>
        ))}
      </div>
      <div className="mt-3 flex items-center gap-2">
        <button
          type="button"
          onClick={addRow}
          className="rounded-md bg-slate-800 px-3 py-1.5 text-sm font-medium text-slate-300 transition-colors hover:bg-slate-700"
        >
          + Ligne
        </button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={running}
          className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500 disabled:opacity-50"
        >
          {running ? 'En cours…' : 'Exécuter le batch'}
        </button>
      </div>
      {error && <p className="mt-2 text-xs text-red-400">Erreur : {error}</p>}
      {results && (
        <table className="mt-3 w-full border-collapse font-mono text-xs">
          <thead>
            <tr className="text-slate-400">
              <th className="border-b border-slate-800 py-1 text-left">#</th>
              <th className="border-b border-slate-800 py-1 text-left">Résultat</th>
            </tr>
          </thead>
          <tbody>
            {results.map((r, i) => (
              <tr key={i} className="text-slate-300">
                <td className="border-b border-slate-800/60 py-1">{i + 1}</td>
                <td className="border-b border-slate-800/60 py-1">
                  {r.error ? <span className="text-red-400">Erreur : {r.error}</span> : r.value || 'OK'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}
