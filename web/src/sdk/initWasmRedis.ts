// SDK TypeScript typé, lot 3 du cahier des charges (§5) : masque le worker
// + le protocole postMessage derrière une API fluide, purement fonctionnelle
// (factory de closures, aucune classe, aucun `new`, aucun `this`).
//
// Écart déliberé par rapport à l'exemple du cahier (`age: number`, valeurs
// non-string) : le moteur GoRedis est un store clé -> string, sans notion
// de "champ" au sein d'une valeur (cf. instruction/roadmap-redis-wasm.md,
// qui clarifie que seul le champ "value" existe pour GET WHERE). Schema type
// donc l'ENSEMBLE DES CLÉS valides et leur valeur (toujours string) — comme
// un vrai client Redis (ioredis, node-redis : GET renvoie toujours une
// string, même pour une valeur numérique stockée par l'appelant). Aucune
// dépendance runtime n'est nécessaire : tout le typage est inféré depuis
// Schema, un paramètre de type pur.
import { createWasmClient } from '../client/wasmClient'
import type { BatchResult, FilterOp, GoRedisClient, Match } from '../client/types'

// GoRedisKeyNotFoundError : erreur typée renvoyée par get()/delete() quand
// la clé est absente (ou expirée, cf. TTL) — à distinguer d'une erreur de
// transport (Worker crashé, etc.) par `instanceof`, sans parser de message.
export class GoRedisKeyNotFoundError extends Error {
  readonly key: string
  constructor(key: string) {
    super(`Clé absente : ${key}`)
    this.name = 'GoRedisKeyNotFoundError'
    this.key = key
  }
}

function toTypedError(err: unknown, key: string): Error {
  if (err instanceof Error && err.message.includes('absente')) {
    return new GoRedisKeyNotFoundError(key)
  }
  return err instanceof Error ? err : new Error(String(err))
}

export interface SetOptions {
  // TTL en secondes (SET ... EX <ex>). Absent = pas de TTL, la clé reste
  // persistante.
  ex?: number
}

export interface Entry<Schema extends Record<string, string>> {
  key: keyof Schema & string
  value: string
}

// QueryBuilder : chaque .where() ajoute un filtre indépendant, exécuté en
// parallèle contre le moteur (une requête GET WHERE par filtre — le moteur
// ne connaît qu'un seul prédicat à la fois) puis combiné en ET côté SDK par
// intersection des clés. Un seul aller-retour n'aurait pas de sens ici :
// GetWhere côté Go n'accepte qu'un (op, valeur), jamais une conjonction.
export interface QueryBuilder<Schema extends Record<string, string>> {
  where(op: FilterOp, value: string): QueryBuilder<Schema>
  exec(): Promise<Entry<Schema>[]>
}

function intersectByKey(resultSets: Match[][]): Match[] {
  if (resultSets.length === 0) return []
  const [first, ...rest] = resultSets
  const restKeySets = rest.map((set) => new Set(set.map((m) => m.key)))
  return first.filter((m) => restKeySets.every((keys) => keys.has(m.key)))
}

function makeQueryBuilder<Schema extends Record<string, string>>(client: GoRedisClient): QueryBuilder<Schema> {
  const filters: { op: FilterOp; value: string }[] = []
  const builder: QueryBuilder<Schema> = {
    where(op, value) {
      filters.push({ op, value })
      return builder
    },
    async exec() {
      if (filters.length === 0) return []
      const resultSets = await Promise.all(filters.map((f) => client.query(f.op, f.value)))
      return intersectByKey(resultSets).map((m) => ({ key: m.key as keyof Schema & string, value: m.value }))
    },
  }
  return builder
}

// SdkBatchCommand : forme produite par db.cmd.set/get/delete, consommée par
// db.batch(). Fermée sur Schema pour que les clés du batch soient, elles
// aussi, vérifiées à la compile.
export type SdkBatchCommand<Schema extends Record<string, string>> =
  | { op: 'set'; key: keyof Schema & string; value: string; ex?: number }
  | { op: 'get'; key: keyof Schema & string }
  | { op: 'delete'; key: keyof Schema & string }

export interface CommandFactory<Schema extends Record<string, string>> {
  set<K extends keyof Schema & string>(key: K, value: Schema[K], opts?: SetOptions): SdkBatchCommand<Schema>
  get<K extends keyof Schema & string>(key: K): SdkBatchCommand<Schema>
  delete<K extends keyof Schema & string>(key: K): SdkBatchCommand<Schema>
}

export interface WasmRedis<Schema extends Record<string, string>> {
  set<K extends keyof Schema & string>(key: K, value: Schema[K], opts?: SetOptions): Promise<void>
  get<K extends keyof Schema & string>(key: K): Promise<Schema[K]>
  get(): QueryBuilder<Schema>
  delete<K extends keyof Schema & string>(key: K): Promise<void>
  batch(commands: SdkBatchCommand<Schema>[]): Promise<BatchResult[]>
  cmd: CommandFactory<Schema>
  dispose(): void
}

// initWasmRedis : point d'entrée du SDK (§5.1). Charge le Worker + le WASM
// (délégué à createWasmClient, même transport que le dashboard React — pas
// de code dupliqué) et renvoie un objet de fonctions typé par Schema.
export async function initWasmRedis<Schema extends Record<string, string>>(): Promise<WasmRedis<Schema>> {
  const { client } = createWasmClient()

  const get = ((key?: string) => {
    if (key === undefined) return makeQueryBuilder<Schema>(client)
    return client.get(key).catch((err: unknown) => {
      throw toTypedError(err, key)
    })
  }) as WasmRedis<Schema>['get']

  const cmd: CommandFactory<Schema> = {
    set(key, value, opts) {
      return { op: 'set', key, value: String(value), ex: opts?.ex }
    },
    get(key) {
      return { op: 'get', key }
    },
    delete(key) {
      return { op: 'delete', key }
    },
  }

  return {
    async set(key, value, opts) {
      await client.set(key, String(value), opts?.ex)
    },
    get,
    async delete(key) {
      try {
        await client.delete(key)
      } catch (err) {
        throw toTypedError(err, key)
      }
    },
    async batch(commands) {
      return client.batch(
        commands.map((c) => ({
          op: c.op,
          key: c.key,
          value: c.op === 'set' ? c.value : undefined,
          ttlSeconds: c.op === 'set' ? c.ex : undefined,
        })),
      )
    },
    cmd,
    dispose() {
      client.dispose()
    },
  }
}
