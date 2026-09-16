import { useState } from 'react'
import type { GoRedisClient } from '../client/types'

interface BenchResult {
  operations: number
  setP50: number
  setP95: number
  getP50: number
  getP95: number
  totalMs: number
}

interface BatchBenchResult {
  operations: number
  individualMs: number
  batchMs: number
  speedup: number
}

function percentile(values: number[], p: number): number {
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length))
  return sorted[idx]
}

async function runBenchmark(client: GoRedisClient, n: number): Promise<BenchResult> {
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
    operations: n,
    setP50: percentile(setTimes, 50),
    setP95: percentile(setTimes, 95),
    getP50: percentile(getTimes, 50),
    getP95: percentile(getTimes, 95),
    totalMs: performance.now() - start,
  }
}

// runBatchGain chiffre le §9.2 "Gain du batch" : N commandes envoyées une
// par une (N aller-retours transport) vs les mêmes N commandes en un seul
// appel client.batch() (1 aller-retour). Des clés dédiées ("benchbatch:*")
// pour ne pas polluer les mesures SET/GET de runBenchmark.
async function runBatchGain(client: GoRedisClient, n: number): Promise<BatchBenchResult> {
  const individualStart = performance.now()
  for (let i = 0; i < n; i++) {
    await client.set(`benchbatch:individual:${i}`, `v${i}`)
  }
  const individualMs = performance.now() - individualStart

  const commands = Array.from({ length: n }, (_, i) => ({
    op: 'set' as const,
    key: `benchbatch:batched:${i}`,
    value: `v${i}`,
  }))
  const batchStart = performance.now()
  await client.batch(commands)
  const batchMs = performance.now() - batchStart

  return { operations: n, individualMs, batchMs, speedup: individualMs / Math.max(batchMs, 0.001) }
}

// BenchmarkPanel mesure SET/GET séquentiels (p50/p95) et le gain du batch
// sur le moteur WASM, en conditions réelles de navigateur.
export function BenchmarkPanel({ client }: { client: GoRedisClient }) {
  const [operations, setOperations] = useState(200)
  const [running, setRunning] = useState(false)
  const [results, setResults] = useState<BenchResult[]>([])
  const [batchResults, setBatchResults] = useState<BatchBenchResult[]>([])

  async function handleRun() {
    setRunning(true)
    try {
      const result = await runBenchmark(client, operations)
      setResults((prev) => [...prev, result])
    } finally {
      setRunning(false)
    }
  }

  async function handleRunBatchGain() {
    setRunning(true)
    try {
      const result = await runBatchGain(client, operations)
      setBatchResults((prev) => [...prev, result])
    } finally {
      setRunning(false)
    }
  }

  return (
    <section className="rounded-lg border border-slate-800 bg-slate-900/60 p-4">
      <h2 className="mb-3 text-sm font-semibold text-slate-300">Benchmark (WASM)</h2>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <input
          type="number"
          min={10}
          max={5000}
          value={operations}
          onChange={(e) => setOperations(Number(e.target.value))}
          className="w-24 rounded-md border border-slate-700 bg-slate-950 px-2 py-1.5 text-sm text-slate-200 focus:border-violet-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={handleRun}
          disabled={running}
          className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500 disabled:opacity-50"
        >
          {running ? 'En cours…' : 'Benchmark SET/GET'}
        </button>
        <button
          type="button"
          onClick={handleRunBatchGain}
          disabled={running}
          className="rounded-md bg-violet-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-violet-500 disabled:opacity-50"
        >
          {running ? 'En cours…' : 'Gain batch'}
        </button>
        {results.length > 0 && (
          <button
            type="button"
            onClick={() => setResults([])}
            className="rounded-md bg-slate-800 px-3 py-1.5 text-sm font-medium text-slate-300 transition-colors hover:bg-slate-700"
          >
            Effacer
          </button>
        )}
      </div>
      {results.length > 0 && (
        <table className="mb-3 w-full border-collapse font-mono text-xs">
          <thead>
            <tr className="text-slate-400">
              <th className="border-b border-slate-800 py-1 text-right">N</th>
              <th className="border-b border-slate-800 py-1 text-right">SET p50</th>
              <th className="border-b border-slate-800 py-1 text-right">SET p95</th>
              <th className="border-b border-slate-800 py-1 text-right">GET p50</th>
              <th className="border-b border-slate-800 py-1 text-right">GET p95</th>
              <th className="border-b border-slate-800 py-1 text-right">Total</th>
            </tr>
          </thead>
          <tbody>
            {results.map((r, i) => (
              <tr key={i} className="text-slate-300">
                <td className="border-b border-slate-800/60 py-1 text-right">{r.operations}</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.setP50.toFixed(2)}ms</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.setP95.toFixed(2)}ms</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.getP50.toFixed(2)}ms</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.getP95.toFixed(2)}ms</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.totalMs.toFixed(0)}ms</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {batchResults.length > 0 && (
        <table className="w-full border-collapse font-mono text-xs">
          <thead>
            <tr className="text-slate-400">
              <th className="border-b border-slate-800 py-1 text-right">N</th>
              <th className="border-b border-slate-800 py-1 text-right">N messages individuels</th>
              <th className="border-b border-slate-800 py-1 text-right">1 batch</th>
              <th className="border-b border-slate-800 py-1 text-right">Gain</th>
            </tr>
          </thead>
          <tbody>
            {batchResults.map((r, i) => (
              <tr key={i} className="text-slate-300">
                <td className="border-b border-slate-800/60 py-1 text-right">{r.operations}</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.individualMs.toFixed(0)}ms</td>
                <td className="border-b border-slate-800/60 py-1 text-right">{r.batchMs.toFixed(0)}ms</td>
                <td className="border-b border-slate-800/60 py-1 text-right">×{r.speedup.toFixed(1)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}
