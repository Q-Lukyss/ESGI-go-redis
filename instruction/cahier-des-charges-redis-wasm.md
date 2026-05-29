# Cahier des charges — « WasmRedis »

Base de données clé-valeur de type Redis, écrite en **Go** et compilée en **WebAssembly**, persistée dans le navigateur via **OPFS**, pilotée par un **SDK TypeScript typé** et visualisée dans une **UI React** sans lag.

---

## 1. Objectif

Implémenter un moteur de stockage clé-valeur en mémoire, façon Redis, qui :

- tourne **entièrement dans le navigateur** (pas de serveur),
- est **écrit en Go et compilé en WASM** (`GOOS=js GOARCH=wasm`) — la logique métier vit en Go, pas en JS,
- **persiste sur disque** via l'Origin Private File System (OPFS),
- est **requêtable** depuis du code applicatif via un SDK TypeScript,
- est **observable** via une interface React capable d'afficher l'intégralité de la base sans ralentir.

Le projet est découpé en 4 lots. Les lots 1 et 2 forment le moteur, les lots 3 et 4 la couche cliente.

---

## 2. Architecture générale

```
┌──────────────── Main thread ─────────────────┐      ┌──────── Web Worker ─────────┐
│  UI React  ◄──►  SDK TypeScript (query        │ MSG  │  Moteur Go → WASM           │
│  (visu DB)        builder typé)        ────────┼─────►│  ├─ state  (RAM)            │
│                                                │      │  ├─ buffer (RAM)           │
│                                                │      │  ├─ index B-Tree           │
└────────────────────────────────────────────────┘      │  └─ config (env)           │
                                                         │         │ flush            │
                                                         │         ▼                  │
                                                         │   OPFS (disque)            │
                                                         │   ├─ AOF (append log)      │
                                                         │   └─ snapshot.json         │
                                                         └─────────────────────────────┘
```

**Contrainte importante :** les *sync access handles* d'OPFS ne sont disponibles **que dans un Web Worker**. Le moteur WASM + son I/O disque **doivent donc tourner dans un worker**, et le main thread (React + SDK) communique avec lui par messages.

---

## 3. Lot 1 — Moteur WASM (cœur)

### 3.1 Protocole de commandes

Le moteur reçoit des requêtes texte parsées en commande + arguments :

| Commande | Forme | Effet |
|----------|-------|-------|
| `SET` | `SET <key> "<value>"` | `state[arg1] = arg2` |
| `DELETE` | `DELETE <key>` | supprime la clé |
| `GET` | `GET <key>` | renvoie la valeur |
| `GET` filtré | `GET WHERE <champ> <op> <valeur>` | renvoie les entrées matchant le prédicat |

Opérateurs de filtre à supporter sur `GET` : **`equals`, `contains`, `>`, `>=`, `<`, `<=`**.

### 3.2 Données en mémoire (RAM)

- `state` : le store vivant (map clé → valeur).
- `buffer` : file des opérations d'écriture en attente de persistance. Chaque `SET`/`DELETE` fait un `buffer.push(op)`.

### 3.3 Index B-Tree

Les filtres de plage (`>`, `>=`, `<`, `<=`) **imposent un index ordonné** : implémenter un **B-Tree** (ou B+Tree) maintenu à jour à chaque écriture, utilisé pour résoudre les `GET` filtrés sans scan complet. À construire/charger **au démarrage** à partir des données persistées.

> `equals` / `contains` peuvent être résolus par scan ou index secondaire ; les range queries **doivent** passer par le B-Tree.

### 3.4 TTL / expiration (obligatoire)

- Chaque clé peut recevoir une durée de vie : `SET <key> "<value>" EX <secondes>`.
- Une clé expirée n'est plus visible (`GET`) et doit finir par être réellement supprimée du `state`, du B-Tree et persistée comme une suppression (expiration lazy au `GET` **et** balayage périodique en arrière-plan).
- La durée par défaut et la fréquence de balayage sont des **constantes configurables** (voir §7.2).

### 3.5 Cycle de flush mémoire

- Toutes les **~1 seconde**, un trigger vide le `buffer` vers la couche de persistance (lot 2).
- **Une seule opération de flush à la fois** (verrou / mutex) : pas de flush concurrent.

### 3.6 Batch de commandes (obligatoire)

- Le moteur doit accepter **un lot de plusieurs commandes en un seul appel** (un seul `postMessage` worker), les exécuter dans l'ordre et renvoyer un **tableau de résultats** aligné sur les commandes.
- Objectif : amortir le coût de l'aller-retour worker (sérialisation + `postMessage`) quand on enchaîne beaucoup d'écritures/lectures. Le gain est à chiffrer dans le benchmark (N commandes en N messages vs 1 batch).
- Le batch est atomique au sens « exécuté d'un bloc côté moteur » (pas d'interleaving avec d'autres messages), mais **sans rollback** : ce n'est pas une transaction.

---

## 4. Lot 2 — Persistance OPFS

Deux mécanismes complémentaires, comme Redis (AOF + RDB).

### 4.1 AOF — « Buffer persistant » (append-only log)

- À chaque flush (≈1s), les opérations du buffer sont **ajoutées en append** dans un fichier log OPFS (`aof.log`).
- Garantit la **durabilité** : aucune écriture acquittée n'est perdue en cas de fermeture d'onglet.
- **Anti-collision multi-écriture** : un seul writer à la fois sur le fichier (exclusivité via le sync access handle OPFS, ou Web Locks API). Toute tentative d'écriture concurrente doit être sérialisée, jamais corrompre le fichier.

### 4.2 Snapshot — « State persistant » (RDB-like)

- Toutes les **~2 minutes** : dump complet de `state` dans `snapshot.json` (ex. `{ "name": "matt", ... }`), puis **compaction** : l'AOF est vidé (les opérations antérieures sont désormais couvertes par le snapshot).
- Réduit le temps de restore et la taille de l'AOF.

### 4.3 Restore au démarrage

Au lancement du moteur :

1. Charger `snapshot.json` → reconstruire `state`.
2. **Rejouer** les opérations de l'AOF postérieures au snapshot.
3. (Re)construire l'index B-Tree.

L'état restauré doit être **strictement identique** à l'état avant fermeture.

---

## 5. Lot 3 — SDK TypeScript

Une lib cliente qui masque la communication worker/WASM et expose une API ergonomique, **typée** et **purement fonctionnelle** (pas de classe, pas de `new` — une factory `initWasmRedis` qui renvoie un objet de fonctions).

### 5.1 Query builder typé (génériques)

- Point d'entrée : `initWasmRedis<Schema>()` (asynchrone, charge le WASM et le worker).
- API fluide et chaînable, basée sur des closures (aucun `this`, aucune classe).
- Typage par **génériques** : le schéma de la base type les clés, les champs filtrables et les valeurs de retour.

Exemple de la cible visée :

```ts
type Schema = { name: string; age: number };

const db = await initWasmRedis<Schema>();

await db.set("name", "matt");

// query builder typé : "age" force un number, l'op est validé à la compile
const res = await db
  .get()
  .where("age", ">", 18)
  .where("name", "contains", "ma")
  .exec(); // res: Entry<Schema>[]

// batch : plusieurs commandes en un seul aller-retour worker (cf. §3.6)
await db.batch([
  db.cmd.set("a", "1"),
  db.cmd.delete("old"),
  db.cmd.set("b", "2", { ex: 60 }),
]);
```

### 5.2 Exigences

- `set`, `delete`, `get` (clé directe + builder filtré), `batch`.
- Sérialisation/désérialisation et transport vers le worker invisibles pour l'utilisateur.
- Erreurs typées (clé absente, opérateur invalide…).
- Aucune dépendance runtime requise pour le typage (tout doit être inféré).

---

## 6. Lot 4 — UI React (visualisation)

Interface d'inspection et d'édition de la base, **performante sur très gros volumes**.

### 6.1 Fonctionnel

- Afficher toutes les entrées de la base (clé / valeur).
- **Ajouter**, **modifier**, **supprimer** une entrée (CRUD branché sur le SDK).

### 6.2 Virtual scroll — codé à la main (obligatoire)

- **Aucune librairie** de virtualisation (pas de react-window/virtuoso/etc.).
- Ne monter dans le DOM que les lignes visibles (+ une petite marge), positionnement absolu + spacer pour la hauteur totale, calcul de `startIndex`/`endIndex` au scroll.
- **Critère** : scroll fluide de bout en bout sur une base de **≥ 100 000 entrées** (viser 1 M), sans lag perceptible.

### 6.3 Render fin — « déjouer » React (obligatoire)

- Une modification d'une ligne ne doit **re-render que cette ligne**, pas l'ensemble des lignes affichées.
- À démontrer (ex. React DevTools « highlight updates » : seule la ligne changée doit clignoter, ou un compteur de render par ligne).
- Pistes attendues : store externe avec abonnement par ligne (`useSyncExternalStore` + sélecteur ciblé), pub/sub par clé, ou mutation DOM imperative ciblée — `React.memo` seul ne suffit pas.

---

## 7. Contraintes techniques

### 7.1 Stack & exécution

- **Go → WASM** : le moteur (state, buffer, B-Tree, TTL, parsing des commandes, persistance) est écrit en **Go** et compilé en WebAssembly (`GOOS=js GOARCH=wasm`). La logique métier ne doit pas vivre en JS.
- **Worker** : moteur + OPFS isolés dans un Web Worker ; UI jamais bloquée.
- **OPFS** : accès via `navigator.storage.getDirectory()` et `createSyncAccessHandle()` (worker).
- **Zéro serveur** : tout en local navigateur.
- **TypeScript strict** côté SDK et UI.

### 7.2 Configuration par environnement (obligatoire)

- **Aucune constante codée en dur** : toute valeur de configuration (intervalle de flush ~1s, intervalle de snapshot ~2min, TTL par défaut, fréquence de balayage des expirations, ordre du B-Tree, taille de page du virtual scroll, tailles de buffer, etc.) doit être **surchargeable via un fichier d'environnement** (`.env` / `config.env`).
- Fournir un `.env.example` documentant chaque variable, sa valeur par défaut et son rôle.

### 7.3 Style de code & qualité d'architecture (évalué)

- Architecture **la plus propre possible** : séparation nette moteur / persistance / transport / SDK / UI.
- Favoriser **1 fonction = 1 fichier** quand c'est pertinent, nommage explicite, responsabilités isolées.
- Code lisible, testable, sans logique dupliquée.
- **Style fonctionnel, pas impératif** : privilégier les transformations déclaratives. Côté Go, **utiliser `samber/lo` autant que possible** (`Map`, `Filter`, `Reduce`, `GroupBy`…) plutôt que des boucles `for` à la main.
- **Immutabilité par défaut** : éviter les mutations en place. Les seules mutations tolérées sont celles **réellement justifiées par la performance à un endroit précis** (ex. hot path d'écriture, structure du B-Tree, buffer de flush) — et elles doivent alors être **explicitement commentées/justifiées** dans le code.
- Packages Go supplémentaires autorisés s'ils ont du sens : helpers fonctionnels (`samber/lo`), **monades** (`samber/mo` : `Option`, `Result`, `Either`), etc. À utiliser avec discernement, pas pour faire joli.

### 7.4 Validation des entrées (obligatoire)

- **Toute donnée venant du frontend doit être parsée/validée contre un schéma** au passage de la frontière (UI → worker → Go) : rejet propre des entrées malformées, jamais d'entrée brute non vérifiée dans le moteur.
- Côté TS : validation par schéma (ex. zod ou équivalent) avant l'envoi au worker.
- Côté Go : revalidation/parsing des messages reçus avant exécution (ne jamais faire confiance à l'émetteur).

### 7.5 Optimisations (encouragées)

- **Toute tentative d'optimisation est la bienvenue** et valorisée : structures mémoire, sérialisation, réduction des allocations Go, encodage binaire de l'AOF, batching des messages worker, etc. À documenter et, idéalement, à chiffrer dans le benchmark (§9).

### 7.6 Outillage & Docker (optionnel)

- **Docker autorisé** dans le repo si l'équipe le souhaite (reproductibilité de l'environnement de build/dev).
- **Attention à Air (live-reload Go) selon l'OS** : le file-watching à travers un bind mount Docker est peu fiable sur **macOS/Windows** (les événements `inotify` ne se propagent pas via VirtioFS/gRPC-FUSE) → activer le **mode polling** (`poll = true` dans `.air.toml`) ou rester sur Linux natif.
- De plus, **Air n'est pas idéal pour une cible `GOOS=js GOARCH=wasm`** (il relance un binaire, ne recharge pas le navigateur). Approche recommandée : un **watcher côté JS (Vite/esbuild)** qui déclenche le rebuild du `.wasm` puis le reload du navigateur.

---

## 8. Livrables

**Tout est déposé dans un dépôt GitHub partagé avec `matteodelandhuydev@gmail.com`.** Le dépôt contient :

1. Code source du moteur **Go** + build WASM reproductible (script de build documenté).
2. SDK TypeScript packagé + types exportés.
3. Application React de visualisation.
4. `.env.example` listant toutes les constantes configurables.
5. README : commandes de build/run, schéma d'architecture, choix techniques (B-Tree, TTL, anti-collision, stratégie de render fin, optimisations).
6. Jeu de données de démo (≥ 100 000 entrées) pour tester la perf.
7. **Rapport de benchmark** complet (voir §9.2).
8. **Le support de présentation** (diapo) versionné dans le repo.

### 8.1 Présentation orale (15 minutes)

- Pitch de **15 minutes** avec **diapo de qualité « conférence open source »** : claire, soignée, qui scotche par le résultat **et** par l'explication du mécanisme.
- **Très précise techniquement** : chaque membre doit maîtriser sa codebase et savoir expliquer le fonctionnement interne (cycle SET → buffer → flush → AOF → snapshot → restore, fonctionnement du B-Tree, du TTL, du virtual scroll, du render granulaire).
- **Appuyée sur des schémas** : l'architecture, les flux de données et les mécanismes doivent être illustrés, pas seulement décrits.
- Objectif : qu'à la fin, l'auditoire comprenne **parfaitement** quoi et comment.

---

## 9. Critères d'évaluation

### 9.1 Grille

| Axe | Attendu |
|-----|---------|
| Moteur Go | SET/DELETE/GET + filtres fonctionnels, parsing correct, compilé en WASM |
| Batch | exécution d'un lot de commandes en un seul aller-retour worker |
| TTL | expiration lazy + balayage, suppression réelle et persistée |
| B-Tree | range queries résolues par l'index, pas de full scan |
| Persistance | AOF + snapshot, restore identique après reload/crash |
| Concurrence | écritures sérialisées, fichier jamais corrompu |
| Config | aucune constante en dur, tout surchargeable via env |
| Architecture | propre, modulaire, 1 fonction/1 fichier, lisible |
| Style | fonctionnel (samber/lo), immutabilité par défaut, mutations justifiées seulement |
| Validation | entrées frontend parsées/validées par schéma à la frontière |
| SDK | query builder réellement typé (erreurs détectées à la compile) |
| Virtual scroll | fait main, fluide sur ≥ 100k lignes |
| Render fin | seule la ligne modifiée re-render (démontré) |
| Benchmark | mesures chiffrées et reproductibles (§9.2) |
| Présentation | diapo qualité conf, maîtrise tech, schémas clairs |

### 9.2 Benchmark obligatoire

Rapport chiffré et reproductible, comprenant au minimum :

**Moteur Go / WASM**
- Latence **SET** (p50 / p95).
- Latence **GET** par clé (p50 / p95).
- Latence **GET filtré** (equals, contains, et range via B-Tree) selon la taille de base.
- **Temps de restore** (snapshot seul, puis snapshot + replay AOF).
- **Gain du batch** : N commandes envoyées une par une vs en un seul batch (débit + latence totale).

**UI React**
- **FPS au scroll** sur la base de démo (≥ 100k entrées).
- Vérification **du render granulaire** : preuve que la modification d'une ligne ne re-render **que** cette ligne (mesure du nombre de renders, capture React DevTools, ou compteur par ligne) — réponse claire oui/non.

Préciser la méthodo (machine, navigateur, taille de base, nombre d'itérations).

---

## 10. Bonus (facultatif)

- **Pub/sub** (abonnement à des clés/patterns, notification sur changement).
- **Encodage binaire** de l'AOF et du snapshot (à la place du JSON) — gain attendu surtout sur le **temps de restore** et le volume d'I/O sur grosse base ; à valider par comparaison chiffrée JSON vs binaire dans le benchmark.
- Synchro multi-onglets (BroadcastChannel + Web Locks).
