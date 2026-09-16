import { useState, type FormEvent } from 'react'
import type { GoRedisClient } from '../client/types'

const inputClass =
  'rounded-md border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-sm text-slate-200 placeholder:text-slate-500 focus:border-violet-500 focus:outline-none'

export function CrudPanel({ client }: { client: GoRedisClient }) {
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [ttl, setTtl] = useState('')
  const [status, setStatus] = useState<string | null>(null)

  async function handleSet(e: FormEvent) {
    e.preventDefault()
    if (!key) return
    const ttlSeconds = ttl ? Number(ttl) : undefined
    try {
      await client.set(key, value, ttlSeconds)
      setStatus(ttlSeconds ? `SET ${key} = "${value}" (EX ${ttlSeconds}s) OK` : `SET ${key} = "${value}" OK`)
    } catch (err) {
      setStatus(`Erreur : ${(err as Error).message}`)
    }
  }

  async function handleDelete() {
    if (!key) return
    try {
      await client.delete(key)
      setStatus(`DELETE ${key} OK`)
    } catch (err) {
      setStatus(`Erreur : ${(err as Error).message}`)
    }
  }

  return (
    <form className="flex flex-wrap items-center gap-2 rounded-lg border border-slate-800 bg-slate-900/60 p-4" onSubmit={handleSet}>
      <input className={inputClass} placeholder="clé" value={key} onChange={(e) => setKey(e.target.value)} />
      <input className={inputClass} placeholder="valeur" value={value} onChange={(e) => setValue(e.target.value)} />
      <input
        placeholder="TTL (s, optionnel)"
        type="number"
        min="1"
        value={ttl}
        onChange={(e) => setTtl(e.target.value)}
        className={`${inputClass} w-36`}
      />
      <button type="submit" className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500">
        SET
      </button>
      <button
        type="button"
        onClick={handleDelete}
        className="rounded-md bg-slate-800 px-3 py-1.5 text-sm font-medium text-slate-300 transition-colors hover:bg-slate-700"
      >
        DELETE
      </button>
      {status && <span className="text-xs text-slate-400">{status}</span>}
    </form>
  )
}
