import { useState } from 'react'

interface SeedPanelProps {
  seed: (count: number, spreadHours: number) => Promise<{ stateCount: number }>
}

// SeedPanel n'a de sens qu'avec le backend WASM : un process natif ne peut
// pas écrire dans l'OPFS d'un onglet, donc le peuplement de démo doit être
// déclenché depuis le navigateur lui-même (cf. infrastructure/wasmbridge).
export function SeedPanel({ seed }: SeedPanelProps) {
  const [count, setCount] = useState(100_000)
  const [status, setStatus] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function handleSeed() {
    setLoading(true)
    setStatus('Peuplement en cours…')
    try {
      const t0 = performance.now()
      const res = await seed(count, 24 * 30)
      setStatus(`${res.stateCount.toLocaleString('fr-FR')} clés en state — ${(performance.now() - t0).toFixed(0)}ms`)
    } catch (err) {
      setStatus(`Erreur : ${(err as Error).message}`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="seed-panel">
      <input
        type="number"
        min={1}
        max={2_000_000}
        value={count}
        onChange={(e) => setCount(Number(e.target.value))}
      />
      <button type="button" onClick={handleSeed} disabled={loading}>
        {loading ? 'Peuplement…' : 'Peupler (démo)'}
      </button>
      {status && <span className="seed-status">{status}</span>}
    </div>
  )
}
