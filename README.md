# GoRedis

Base de données clé-valeur façon Redis, écrite en **Go**, compilée en
**WebAssembly** et exécutée dans un **Web Worker** du navigateur, avec
persistance via **OPFS** (`navigator.storage`) — zéro serveur, conforme au
cahier des charges (`instruction/cahier-des-charges-redis-wasm.md`).

Le même moteur (`core/`) tourne aussi en natif via un REPL, pour une
utilisation en ligne de commande sans navigateur.

## Sommaire

- [Démarrage rapide](#démarrage-rapide)
- [Architecture](#architecture)
- [Commandes du moteur](#commandes-du-moteur)
- [SDK TypeScript](#sdk-typescript)
- [Configuration](#configuration)
- [Choix techniques](#choix-techniques)
- [Tests et benchmarks](#tests-et-benchmarks)

## Démarrage rapide

Prérequis : Go 1.26+, Node 18+.

### REPL natif

```bash
go run ./cmd/repl
```

Édition de ligne, historique persistant (`.goredis_history`) et complétion
des mots-clés via [`chzyer/readline`](https://github.com/chzyer/readline).

### Dashboard WASM (Worker + OPFS)

Le binaire WASM est un artefact de build (`web/public/goredis.wasm`, gitignoré) :
à reconstruire après toute modification de `core/`, `infrastructure/wasmbridge`
ou `infrastructure/storage/opfs`.

```bash
GOOS=js GOARCH=wasm go build -o web/public/goredis.wasm ./cmd/wasm
cd web && npm install && npm run dev
```

Puis dans le dashboard, cliquer "Peupler (démo)" pour générer un jeu de
données de test.

### Peupler une base de démo pour le REPL natif (≥ 100 000 entrées)

```bash
go run ./cmd/seed --count 1000000 --spread 720h
go run ./cmd/repl
```

`cmd/seed` génère un jeu de données reproductible (graine fixe) directement
dans le format de persistance du moteur (un seul `Snapshot()` final, pas
d'écriture clé par clé) — 1M clés en ~10s sur une machine de développement
courante. Le dashboard WASM se peuple depuis le navigateur lui-même (bouton
"Peupler"), un process natif ne pouvant pas écrire dans l'OPFS d'un onglet.

## Architecture

```
core/                    moteur pur (state, buffer, B-Tree, TTL, persistance)
                         compile natif ET en WASM sans modification
infrastructure/
  ├─ storage/file        Storage sur disque (REPL natif)
  ├─ storage/opfs        Storage sur OPFS (backend WASM, syscall/js)
  ├─ wasmbridge          protocole postMessage (CRUD, query, batch, browse, stats)
  └─ repl                parsing texte + REPL (SET/GET/DELETE/GET WHERE, EX, batch)
cmd/
  ├─ repl                binaire natif : REPL sur infrastructure/storage/file
  ├─ wasm                composition root WASM (//go:build js && wasm)
  └─ seed                génère un jeu de données de démo, natif
web/
  ├─ src/client           GoRedisClient : abstraction devant wasmClient.ts
  ├─ src/sdk               SDK TypeScript typé (initWasmRedis<Schema>, lot 3)
  ├─ src/components         dashboard (CRUD, query, scroll infini, benchmark)
  └─ public/goredis-worker.js   charge le .wasm dans un Worker dédié
```

`core/` ne connaît ni WebSocket ni `syscall/js` : il expose une API Go pure
(`GoRedis.Set/Get/Delete/GetWhere/Execute/ExecuteBatch`) et une interface
`Storage` pour la persistance. Le natif (`cmd/repl`) et le WASM (`cmd/wasm`)
ne sont que deux compositions différentes de ces mêmes briques.

### Cycle SET → buffer → flush → AOF → snapshot → restore

1. `SET` écrit dans `state` (map en RAM) et pousse l'opération dans
   `opBuffer` (`core/persistence.go`).
2. Toutes les `FlushInterval` (défaut 1s), `Flush()` vide `opBuffer` vers
   l'AOF (append-only, `Storage.AppendAOF`) sous verrou (un seul flush à la
   fois).
3. Toutes les `SnapshotInterval` (défaut 2min), `Snapshot()` écrit une photo
   complète du `state` (avec timestamp et TTL par clé) puis vide l'AOF
   (compaction : l'AOF ne couvre plus que ce qui n'est pas dans le
   snapshot).
4. Au démarrage, `Restore()` charge le snapshot puis rejoue les opérations
   de l'AOF par-dessus (celles postérieures à la dernière photo), puis purge
   les clés dont le TTL a expiré pendant l'arrêt.

### TTL

`SET <clé> "<valeur>" EX <secondes>` pose une expiration (unix-nano) sur la
clé. Deux mécanismes complémentaires (`core/ttl.go`) :

- **lazy** : `Get()` vérifie l'expiration avant de lire, et supprime pour de
  bon (state + index + AOF) si dépassée.
- **actif** : `RunExpirySweepLoop` balaie périodiquement `expireAt` et
  supprime toute clé jamais relue après son expiration.

Un `SET` sans `EX` efface un TTL précédent (comme dans Redis).

### B-Tree

`core/btree.go` implémente un B-Tree générique (degré configurable) qui
indexe `valeur → ensemble de clés`, maintenu à jour à chaque écriture/
suppression. Les requêtes range (`>`, `>=`, `<`, `<=`) le parcourent
(`Range`) sans jamais scanner l'intégralité du store. Le même B-Tree sert
aussi d'index par clé (`keyIndex`) et par timestamp (`timeIndex`) pour la
pagination par curseur du scroll infini (`RangeFrom`) — `equals` passe par
un index inversé dédié (O(1)), `contains` par un scan (pas d'index adapté à
une recherche de sous-chaîne).

### Anti-collision (AOF)

Le fichier AOF n'a qu'un seul écrivain possible à la fois : `Flush()` et
`Snapshot()` sont sérialisés par le mutex du moteur (`GoRedis.mu`), jamais
concurrents entre eux. Côté OPFS, `createSyncAccessHandle` impose de toute
façon l'exclusivité au niveau du navigateur.

### Virtual scroll et rendu fin (React)

- **Scroll infini fait main** (`web/src/components/InfiniteList.tsx`) :
  pagination par curseur B-Tree, positionnement absolu + spacer, aucune
  librairie de virtualisation. Vérifié fluide à 1M lignes.
- **Rendu fin** (`web/src/store/rowStore.ts`) : chaque ligne s'abonne à sa
  propre clé via `useSyncExternalStore` (`KeyedStore`) — une mise à jour ne
  notifie que la ligne concernée, jamais les autres. Un compteur de rendu
  par ligne (visible dans l'UI, `×N`) le démontre en direct.

## Commandes du moteur

Syntaxe REPL (`infrastructure/repl`), identique à ce que le WASM construit
directement en `core.Command` :

```
SET <clé> "<valeur>"                          définit une clé
SET <clé> "<valeur>" EX <secondes>            idem, avec expiration
DELETE <clé>                                  supprime une clé
GET <clé>                                     lit une clé
GET WHERE value equals "<valeur>"             valeur exacte (index inversé)
GET WHERE value contains "<sous-chaîne>"      sous-chaîne (scan)
GET WHERE value >|>=|<|<= <valeur>            range (B-Tree)
cmd1 ; cmd2 ; ...                             batch texte (REPL)
```

WASM (`infrastructure/wasmbridge`, protocole `postMessage`) : mêmes
opérations (`set`, `get`, `delete`, `query`, `batch`, `browse`, `stats`,
`subscribe`, `seed`), corrélées par `requestId`.

## SDK TypeScript

`web/src/sdk/initWasmRedis.ts` (lot 3 du cahier des charges) : factory de
closures typée par générique, aucune classe :

```ts
type Schema = { name: string; age: string }

const db = await initWasmRedis<Schema>()
await db.set('name', 'matt')

const res = await db.get().where('contains', 'ma').where('>=', 'a').exec()
// res: Entry<Schema>[]

await db.batch([
  db.cmd.set('note', 'hello'),
  db.cmd.delete('old'),
  db.cmd.set('age', '31', { ex: 60 }),
])
```

**Écart assumé par rapport à l'exemple du cahier des charges** : `Schema`
type l'ensemble des **clés** valides (et leur valeur, toujours `string`),
pas des "champs" au sein d'une valeur — le moteur est un store clé→string
plat, sans notion de champ (`GET WHERE` filtre sur la valeur entière,
jamais sur un champ nommé, cf. `instruction/roadmap-redis-wasm.md`).
Chaîner plusieurs `.where()` combine les prédicats en **ET**, résolu côté
SDK par intersection des clés (le moteur ne résout qu'un seul prédicat par
requête `GET WHERE`). Détails et justification complète en commentaire de
tête du fichier.

Le dashboard inclut un panneau "Exécuter la démo SDK" qui exécute cette
séquence contre le vrai Worker.

## Configuration

Aucune constante de tuning n'est codée en dur (§7.2 du cahier des
charges) : tout passe par `core.Config` (`core/config.go`,
`core.DefaultConfig()` pour les valeurs par défaut).

- **REPL natif** : lit `.env` à la racine (voir `.env.example`).
- **Dashboard WASM** : un Worker n'a pas de système de fichiers pour lire un
  `.env`. La chaîne de configuration est : `web/.env` (variables `VITE_*`,
  voir `web/.env.example`) → Vite les injecte au build → `wasmClient.ts`
  les passe en query string à l'URL du Worker → `goredis-worker.js` les
  traduit en arguments CLI (`go.argv`) → `cmd/wasm/main.go` les parse
  (`flag`, mêmes noms et mêmes défauts que `core.DefaultConfig()`).

| Variable (.env natif) | Variable (web/.env) | Rôle | Défaut |
|---|---|---|---|
| `WRITE_TO_BUFFER_INTERVAL` (s) | `VITE_FLUSH_INTERVAL_MS` (ms) | cycle de flush AOF | 1s |
| `UPDATE_STATE_FROM_BUFFER_INTERVAL` (min) | `VITE_SNAPSHOT_INTERVAL_MS` (ms) | cycle de snapshot | 2min |
| `EXPIRY_SWEEP_INTERVAL` (s) | `VITE_EXPIRY_SWEEP_INTERVAL_MS` (ms) | fréquence balayage TTL | 10s |
| `DEFAULT_TTL_SECONDS` | `VITE_DEFAULT_TTL_SECONDS` | TTL par défaut (0 = désactivé) | 0 |
| `BTREE_DEGREE` | `VITE_BTREE_DEGREE` | ordre du B-Tree | 3 |
| `CHANGES_BUFFER_SIZE` | `VITE_CHANGES_BUFFER_SIZE` | taille du channel d'évènements | 4096 |
| `BUFFER_FILE` / `STATE_FILE` | `VITE_AOF_FILE` / `VITE_SNAPSHOT_FILE` | noms des fichiers de persistance | voir `.env.example` |
| — | `VITE_PAGE_SIZE` | taille de page du scroll infini | 100 |

## Choix techniques

- **`samber/lo`** utilisé pour les transformations de collections
  (`Map`/`Filter`/`FilterMap`/`Keys`/`Entries`...) plutôt que des boucles
  manuelles, cf. §7.3.
- **`chzyer/readline`** pour le REPL natif (édition de ligne, historique
  persistant, complétion), plutôt qu'une boucle `bufio` minimale.
- **Immutabilité par défaut** ; les mutations en place restent limitées aux
  points chauds explicitement justifiés en commentaire (B-Tree, buffer de
  flush, index).
- **Validation aux frontières** (§7.4) : côté TS (`web/src/client/validate.ts`,
  appelée avant tout envoi transport) et revalidation indépendante côté Go
  (`infrastructure/wasmbridge/validate.go`) — jamais confiance dans
  l'émetteur, même si le client valide déjà.
- **Zéro serveur** (§7.1 du cahier des charges) : un seul backend web, WASM
  dans un Worker, sans process serveur à faire tourner.

## Tests et benchmarks

Le dashboard inclut un panneau de benchmark (SET/GET p50/p95, gain du
batch) contre le moteur WASM en conditions réelles de navigateur.
