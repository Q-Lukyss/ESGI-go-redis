# GoRedis — inventaire des fonctionnalités

Support pour présenter le projet : chaque section = une fonctionnalité
démontrable, avec ce qu'elle fait, où elle vit dans le code, et pourquoi
elle a été construite ainsi (le détail technique à avoir en tête si on te
pose la question).

## 1. Moteur clé-valeur (`core/`)

Store en mémoire (`map[string]string`), écrit en Go, qui compile **sans
modification** en binaire natif (`cmd/repl`, `cmd/seed`) et en WebAssembly
(`cmd/wasm`) — aucune ligne de `core/` ne connaît `syscall/js`, le Worker,
OPFS ou le REPL : il expose uniquement `Set/Get/Delete/GetWhere/Execute/
ExecuteBatch/Browse/Clear` + une interface `Storage` pour la persistance.

- **Pourquoi ça compte** : la même logique (B-Tree, TTL, validation,
  batch...) tourne à l'identique des deux côtés ; un bug corrigé dans
  `core/` est corrigé partout.
- Concurrence : un seul `sync.Mutex` protège state + index ; compteurs
  (`stateCount`, `bufferCount`) en `atomic.Int64` pour être lus sans jamais
  contendre avec le chemin d'écriture (utile pour un ticker de stats).

## 2. Index B-Tree fait main (`core/btree.go`)

Un vrai B-Tree générique (pas une lib externe), degré configurable
(`core.Config.BTreeDegree`), avec `Insert`/`Delete`/`Range`/`RangeFrom` —
insertion/suppression avec split/fusion de nœuds classiques (algorithme de
Cormen), pas juste un arbre trié naïf.

Utilisé à **trois endroits différents** avec la même implémentation :

| Index | Clé de tri | Sert à |
|---|---|---|
| `rangeIndex` | valeur normalisée | `GET WHERE value >\|>=\|<\|<=` (range queries) |
| `keyIndex` | clé elle-même | scroll infini mode "Browse" (ordre alphabétique) |
| `timeIndex` | timestamp (inversé) | scroll infini mode "Activity" (plus récent d'abord) |

Deux astuces à savoir expliquer :
- **`sortKey()`** normalise les valeurs pour que l'ordre lexicographique
  colle à l'ordre numérique (`"10" < "9"` en tri de chaînes brutes sinon) —
  encodage à largeur fixe avec offset.
- **`RangeFrom(cursor, limit)`** ne matérialise jamais plus que `limit`
  éléments, quelle que soit la taille de la base (c'est la brique qui rend
  le scroll infini possible sur 1M+ entrées sans scan complet) : il collecte
  `limit+1` candidats pour savoir s'il reste une page suivante sans aller-
  retour supplémentaire.
- `equals` et `contains` (§3.1) ne passent **pas** par le B-Tree : `equals`
  a son propre index inversé O(1) (`equalsIndex`), `contains` est un scan
  linéaire assumé (pas d'index adapté à une sous-chaîne).

## 3. TTL / expiration (`core/ttl.go`)

`SET <clé> "<valeur>" EX <secondes>` pose une expiration. Deux mécanismes
complémentaires, comme Redis :

- **Lazy** : `Get()` vérifie l'expiration avant de lire, supprime pour de
  bon si dépassée (state + index + AOF).
- **Actif** : `RunExpirySweepLoop` balaie périodiquement `expireAt` en
  arrière-plan — sans ça, une clé expirée jamais relue resterait en
  mémoire pour toujours.
- Un `SET` sans `EX` efface un TTL précédent (comportement Redis standard).
- Fréquence de balayage et TTL par défaut : configurables (`core.Config`),
  jamais codés en dur.

## 4. Persistance AOF + snapshot (`core/persistence.go`)

Deux mécanismes complémentaires, comme Redis (AOF + RDB) :

1. **AOF** (append-only log) : chaque écriture va dans un buffer
   (`opBuffer`), vidé toutes les `FlushInterval` (~1s) vers le disque
   (`Flush()`), sous verrou (un seul flush à la fois).
2. **Snapshot** : toutes les `SnapshotInterval` (~2min), photo complète du
   state écrite sur disque, puis l'AOF est vidé (compaction — les
   opérations passées sont désormais couvertes par la photo).
3. **Restore** au démarrage : charge le snapshot, rejoue l'AOF par-dessus,
   purge les clés expirées pendant l'arrêt.
4. **`Clear()`** (FLUSHALL) : vide state/index/buffer/TTL et persiste
   immédiatement un état vide (nouveau snapshot + AOF vidé).

**Détail piège à connaître** : le verrou (`e.mu`) est relâché *avant*
d'appeler `Storage` dans `Snapshot()`/`Clear()`. Côté WASM, ces appels
attendent une Promise JS (OPFS) — donc rendent la main à la boucle
d'évènements du navigateur. Si le verrou restait tenu, le moindre message
entrant qui en a besoin (ex. `browse`) se bloquerait dessus pendant que la
boucle d'évènements ne peut plus jamais revenir débloquer la Promise :
interblocage réel rencontré et corrigé pendant le développement.

Deux implémentations de `core.Storage`, interchangeables :
- `infrastructure/storage/file` : fichiers disque (REPL natif, `cmd/seed`).
- `infrastructure/storage/opfs` : Origin Private File System du navigateur,
  `syscall/js` + sync access handles (WASM uniquement).

## 5. Backend WASM + Worker (`cmd/wasm`, `infrastructure/wasmbridge`)

Le moteur Go compile en WebAssembly (`GOOS=js GOARCH=wasm`) et tourne
**entièrement dans le navigateur, sans serveur** : chargé dans un Web
Worker dédié (`web/public/goredis-worker.js`), communique avec la page
React par `postMessage`.

- **Pourquoi un Worker et pas le thread principal** : l'OPFS n'expose des
  accès disque synchrones (`createSyncAccessHandle`) que dans un Worker
  dédié — contrainte de la spec, pas un choix du projet.
- **Protocole requête/réponse** (`infrastructure/wasmbridge/protocol.go`) :
  chaque message a un `requestId`, corrélé côté client (`wasmClient.ts`)
  pour retrouver la bonne Promise à résoudre — `postMessage` est nativement
  asynchrone et sans corrélation.
- **Patchs coalescés** : les changements sont regroupés par fenêtre de
  ~250ms (pas un message par écriture) et filtrés par la fenêtre visible
  déclarée par le client (`subscribe` / `filterInWindow`), pour ne jamais
  pousser plus que ce que l'UI affiche.
- **Validation double** : `web/src/client/validate.ts` côté TS avant tout
  envoi, `infrastructure/wasmbridge/validate.go` côté Go avant exécution —
  jamais confiance dans l'émetteur, même si le client valide déjà.
- **Config sans constante en dur** : `web/.env` (Vite, `VITE_*`) →
  query string de l'URL du Worker → arguments CLI (`go.argv`) → `flag`
  dans `cmd/wasm/main.go`.

## 6. REPL natif (`infrastructure/repl`, `cmd/repl`)

CLI en ligne de commande pour piloter le moteur directement (utilisation
locale, sans navigateur), avec un vrai confort d'édition — équivalent de
`rustyline` côté Rust — via [`chzyer/readline`](https://github.com/chzyer/readline) :

- Prompt `goredis> `, édition de ligne (flèches, raccourcis Emacs),
  **historique persistant** entre sessions (`.goredis_history`).
- **Complétion** au Tab sur les mots-clés (`SET`, `GET WHERE value
  equals/contains`, `DELETE`, `FLUSHALL`, `EX`...).
- `Ctrl+C` **et** `Ctrl+D` quittent proprement (comme `redis-cli`).
- Syntaxe supportée : `SET`, `GET`, `GET WHERE value <op> <valeur>`
  (6 opérateurs), `DELETE`, `FLUSHALL`, batch texte (`cmd1 ; cmd2 ; ...`).
- `cmd/repl/main.go` compose le moteur (stockage fichier, `Restore()`, les
  3 boucles de fond flush/snapshot/expiry-sweep) puis lance le REPL —
  totalement indépendant du dashboard WASM (deux moteurs séparés, deux
  stockages séparés : fichiers disque vs OPFS navigateur).

## 7. Batch de commandes (§3.6)

Exécuter plusieurs commandes en **un seul aller-retour transport** plutôt
qu'un message par commande — amortit le coût de sérialisation/`postMessage`.

- Moteur : `GoRedis.ExecuteBatch([]Command) []Result` — exécuté d'un bloc
  (pas d'interleaving avec d'autres messages), mais sans rollback (pas une
  transaction).
- WASM : message `"batch"`, validé entièrement avant exécution (une seule
  commande invalide fait échouer tout le lot, pas d'exécution partielle
  sur un lot malformé).
- REPL : `cmd1 ; cmd2 ; ...` sur une ligne.
- **Frontend** : `BatchPanel.tsx` — UI pour composer plusieurs lignes
  SET/GET/DELETE (avec TTL) et les envoyer en un seul `client.batch()`,
  résultats affichés alignés sur les lignes.
- Mesuré dans `BenchmarkPanel` : gain chiffré (N messages individuels vs
  1 batch) sur le moteur WASM réel.

## 8. Scroll infini fait main (`web/src/components/InfiniteList.tsx`)

Virtualisation **codée à la main**, aucune librairie (pas de
react-window/virtuoso) :

- Ne monte dans le DOM que les lignes visibles + une marge (`overscan`),
  positionnées en absolu (`top` calculé), un spacer porte la hauteur
  totale (`totalCount * rowHeight`) pour que la scrollbar reflète la vraie
  taille du store même avant d'avoir tout chargé.
- Pagination par curseur B-Tree (`RangeFrom`) — jamais un scan/offset O(n).
- Vérifié fluide sur **1M d'entrées** : ~31 lignes montées dans le DOM quel
  que soit le volume total, scroll incrémental sous le budget d'une frame
  à 60fps.
- Deux modes : "Browse" (alphabétique, `keyIndex`) et "Activity"
  (chronologique, `timeIndex`, plus récent d'abord).
- Limite assumée : la pagination séquentielle par curseur ne permet pas un
  vrai "saut à la position N" ; un grand saut de scrollbar affiche un
  placeholder honnête pendant que le chargement séquentiel rattrape.

## 9. Rendu fin — "déjouer" React (`web/src/store/rowStore.ts`, `Row.tsx`)

Une modification d'une ligne ne doit re-render **que cette ligne**, jamais
la liste entière ni les lignes voisines — `React.memo` seul ne suffit pas
(il ne fait que comparer des props après-coup, il faut qu'il n'y ait
structurellement rien à comparer pour les lignes non concernées).

- `KeyedStore<T>` : store externe où chaque ligne s'abonne à **sa propre
  clé** via `useSyncExternalStore` — une mise à jour ne notifie que les
  abonnés de cette clé, les autres composants ne sont même jamais informés
  qu'un changement a eu lieu ailleurs.
- **Démontré en direct** : chaque ligne affiche un compteur `×N` de son
  nombre de rendus (`bumpRenderCount`) — modifier une clé fait uniquement
  progresser le compteur de sa ligne, toutes les autres restent figées.

## 10. SDK TypeScript typé (`web/src/sdk/initWasmRedis.ts`)

Point d'entrée pensé pour être embarqué dans une appli tierce (pas
seulement le dashboard) : factory de closures **purement fonctionnelle**
(aucune classe, aucun `new`, aucun `this`), typée par générique.

```ts
type Schema = { name: string; age: string }
const db = await initWasmRedis<Schema>()
await db.set('name', 'matt')
const res = await db.get().where('contains', 'ma').where('>=', 'a').exec()
await db.batch([db.cmd.set('note', 'hello'), db.cmd.delete('old')])
```

- `Schema` type l'ensemble des **clés** valides et leur valeur (toujours
  `string`, comme un vrai client Redis) — écart assumé par rapport à
  l'exemple du cahier des charges (pas de notion de "champ" dans une
  valeur, le moteur est un store clé→string plat).
- Query builder chaîné : chaque `.where()` interroge le moteur
  indépendamment (une requête `GET WHERE` = un seul prédicat côté Go), la
  conjonction (ET) est résolue **côté SDK** par intersection des clés.
- Erreurs typées (`GoRedisKeyNotFoundError`), détectables par `instanceof`.
- Démontré en direct par `SdkDemoPanel.tsx` contre le vrai Worker+WASM (pas
  un mock).

## 11. Autres commandes / fonctionnalités transverses

- **`GET WHERE`** (`core/index.go`) : 6 opérateurs (`equals`, `contains`,
  `>`, `>=`, `<`, `<=`) — `equals` via index inversé O(1), les 4 opérateurs
  de range via le B-Tree, `contains` par scan (pas d'index adapté à une
  sous-chaîne).
- **`FLUSHALL`** : vide toute la base (REPL, message WASM `"flushall"`,
  bouton "Tout supprimer" dans le dashboard) et persiste immédiatement
  l'état vide.
- **Config par environnement** (§7.2) : aucune constante de tuning codée en
  dur — `core.Config`/`DefaultConfig()`, surchargeable via `.env` (REPL) ou
  `web/.env` (`VITE_*`, WASM).
- **Validation aux frontières** (§7.4) : TS avant tout envoi transport, Go
  revalidé indépendamment côté Worker.
- **Style fonctionnel** : `samber/lo` (`Map`/`Filter`/`FilterMap`/
  `Entries`...) plutôt que des boucles manuelles côté Go.
