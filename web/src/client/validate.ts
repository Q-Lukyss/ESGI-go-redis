// Validation à la frontière UI -> transport (§7.4) : toute donnée saisie
// dans le dashboard est vérifiée ici avant de quitter le navigateur vers le
// Worker WASM — jamais d'entrée brute non contrôlée envoyée au moteur.
// Fonctions pures, sans dépendance (pas besoin de zod pour des règles aussi
// simples : non-vide, longueur bornée, énumération). Le moteur Go revalide
// indépendamment côté Worker (infrastructure/wasmbridge/validate.go) : un
// client ne doit jamais être le seul rempart.
import type { FilterOp } from './types'

export class ValidationError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ValidationError'
  }
}

const MAX_KEY_LENGTH = 1024
const MAX_VALUE_LENGTH = 1_000_000

const FILTER_OPS = new Set<string>(['equals', 'contains', '>', '>=', '<', '<='])

export function validateKey(key: string): void {
  if (key.length === 0) throw new ValidationError('La clé ne peut pas être vide')
  if (key.length > MAX_KEY_LENGTH) throw new ValidationError(`Clé trop longue (max ${MAX_KEY_LENGTH} caractères)`)
}

export function validateValue(value: string): void {
  if (value.length > MAX_VALUE_LENGTH) {
    throw new ValidationError(`Valeur trop longue (max ${MAX_VALUE_LENGTH} caractères)`)
  }
}

export function validateTtl(ttlSeconds: number | undefined): void {
  if (ttlSeconds !== undefined && (ttlSeconds < 0 || !Number.isFinite(ttlSeconds))) {
    throw new ValidationError('Le TTL doit être un nombre de secondes positif ou nul')
  }
}

export function validateFilterOp(op: FilterOp): void {
  if (!FILTER_OPS.has(op)) {
    throw new ValidationError(`Opérateur de filtre invalide : ${op} (attendu : ${[...FILTER_OPS].join(', ')})`)
  }
}
