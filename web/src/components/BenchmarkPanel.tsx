import { useState } from 'react'
import type { GoRedisClient } from '../client/types'

interface BenchResult {
  backend: string
  operations: number
  setP50: number
  setP95: number
  getP50: number
  getP95: number
  totalMs: number
}

function percentile(values: number[], p: number): number {
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length))
  return sorted[idx]
}

async function runBenchmark(client: GoRedisClient, backend: string, n: number): Promise<BenchResult> {
  const setTimes: number[] = []
  const getTimes: number[] = []
  const start = performance.now()

  for (let i = 0; i < n; i++) {
    const key = `bench:${i}`
    const t0 = performance.now()
    await client.set(key, `v${i}`)
    setTimes.push(performance.now() - t0)
  }
  for (let i = 0; i < n; i++) {
    const key = `bench:${i}`
    const t0 = performance.now()
    await client.get(key)
    getTimes.push(performance.now() - t0)
  }

  return {
    backend,
    operations: n,
    setP50: percentile(setTimes, 50),
    setP95: percentile(setTimes, 95),
    getP50: percentile(getTimes, 50),
    getP95: percentile(getTimes, 95),
    totalMs: performance.now() - start,
  }
}

// BenchmarkPanel mesure SET/GET séquentiels (p50/p95) sur le backend
// actuellement sélectionné. Basculer de backend puis relancer permet de
// comparer serveur (REST, aller-retour réseau) vs WASM (postMessage,
// même processus navigateur) — cf. TODO.md pour la méthodologie complète.
export function BenchmarkPanel({ client, backend }: { client: GoRedisClient; backend: string }) {
  const [operations, setOperations] = useState(200)
  const [running, setRunning] = useState(false)
  const [results, setResults] = useState<BenchResult[]>([])

  async function handleRun() {
    setRunning(true)
    try {
      const result = await runBenchmark(client, backend, operations)
      setResults((prev) => [...prev, result])
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="benchmark-panel">
      <div className="benchmark-controls">
        <input
          type="number"
          min={10}
          max={5000}
          value={operations}
          onChange={(e) => setOperations(Number(e.target.value))}
        />
        <button type="button" onClick={handleRun} disabled={running}>
          {running ? 'En cours…' : `Benchmark (${backend})`}
        </button>
        {results.length > 0 && (
          <button type="button" onClick={() => setResults([])}>
            Effacer
          </button>
        )}
      </div>
      {results.length > 0 && (
        <table className="benchmark-table">
          <thead>
            <tr>
              <th>Backend</th>
              <th>N</th>
              <th>SET p50</th>
              <th>SET p95</th>
              <th>GET p50</th>
              <th>GET p95</th>
              <th>Total</th>
            </tr>
          </thead>
          <tbody>
            {results.map((r, i) => (
              <tr key={i}>
                <td>{r.backend}</td>
                <td>{r.operations}</td>
                <td>{r.setP50.toFixed(2)}ms</td>
                <td>{r.setP95.toFixed(2)}ms</td>
                <td>{r.getP50.toFixed(2)}ms</td>
                <td>{r.getP95.toFixed(2)}ms</td>
                <td>{r.totalMs.toFixed(0)}ms</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
