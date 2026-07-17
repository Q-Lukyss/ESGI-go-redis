import { useState, type FormEvent } from 'react'
import type { GoRedisClient } from '../client/types'

export function CrudPanel({ client }: { client: GoRedisClient }) {
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [status, setStatus] = useState<string | null>(null)

  async function handleSet(e: FormEvent) {
    e.preventDefault()
    if (!key) return
    try {
      await client.set(key, value)
      setStatus(`SET ${key} = "${value}" OK`)
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
    <form className="crud-panel" onSubmit={handleSet}>
      <input placeholder="clé" value={key} onChange={(e) => setKey(e.target.value)} />
      <input placeholder="valeur" value={value} onChange={(e) => setValue(e.target.value)} />
      <button type="submit">SET</button>
      <button type="button" onClick={handleDelete}>
        DELETE
      </button>
      {status && <span className="crud-status">{status}</span>}
    </form>
  )
}
