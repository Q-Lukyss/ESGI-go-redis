import { useState } from 'react'

interface SeedPanelProps {
  seed: (count: number, spreadHours: number) => Promise<{ stateCount: number }>
}

// SeedPanel peuple le store WASM depuis le navigateur lui-même : un process
// natif ne peut pas écrire dans l'OPFS d'un onglet.
export function SeedPanel({ seed }: SeedPanelProps) {
  const [count, setCount] = useState(1_000_000)
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
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-slate-800 bg-slate-900/60 p-4">
      <input
        type="number"
        min={1}
        max={2_000_000}
        value={count}
        onChange={(e) => setCount(Number(e.target.value))}
        className="w-32 rounded-md border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-sm text-slate-200 focus:border-violet-500 focus:outline-none"
      />
      <button
        type="button"
        onClick={handleSeed}
        disabled={loading}
        className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500 disabled:opacity-50"
      >
        {loading ? 'Peuplement…' : 'Peupler (démo)'}
      </button>
      {status && <span className="text-xs text-slate-400">{status}</span>}
    </div>
  )
}
