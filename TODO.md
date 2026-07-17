# GoRedis — dashboard React : suivi d'avancement

Contexte complet et décisions d'architecture : voir le plan initial
`~/.claude/plans/harmonic-chasing-papert.md`. Ce fichier ne récapitule que
l'état d'avancement et ce qu'il reste à faire.

Architecture retenue : `core/` (moteur pur, compile natif *et* WASM) +
`infrastructure/` (adaptateurs : stockage, transport) + `cmd/` (binaires) +
`web/` (React, derrière une abstraction `GoRedisClient` interchangeable
serveur/WASM).

## Fait (Phases 0 à 7) — les deux backends + le frontend, vérifiés en conditions réelles

- **Phase 0 — `core/`** : timestamps persistés (`Operation.Timestamp`),
  `keyIndex`/`timeIndex` (BTree réutilisé), pagination par curseur
  (`BTree.RangeFrom`, `GoRedis.Browse`), compteurs atomiques
  (`StateCount`/`BufferCount`) découplés du verrou, flux d'évènements non-
  bloquant (`GoRedis.Changes()`).
- **Phase 1 — `infrastructure/`** : `core/file_storage.go` →
  `infrastructure/storage/file`, parsing texte → `infrastructure/repl`,
  nouveau point d'entrée `cmd/server`.
- **Phase 2 — REST + WebSocket** : `infrastructure/http` (CRUD, `/query`,
  `/browse`, `/stats`), `infrastructure/ws` (hub avec abonnements par
  fenêtre, coalescing ~250ms, patch vs compteur selon insertion/update).
- **Phase 3 — seed natif** : `seed/generator.go` (génération pure) +
  `cmd/seed` (peuple via `SetAt`, un seul `Snapshot()` final). 1M clés en
  ~10,6s.
- **Phase 4 — frontend `web/`** : `GoRedisClient` + `serverClient.ts`,
  scroll infini fait main (pas de lib), rendu fin
  (`useSyncExternalStore` par clé), vues Browse/Activity, stats live, CRUD.
- **Phase 5 — vérification à l'échelle 1M** : seed + restore + `/browse` +
  WS revalidés avec 1 000 000 de clés réelles (pas juste 5 000). Confirmé
  au navigateur (Playwright) : chargement initial 685ms, seulement ~31
  lignes montées dans le DOM quel que soit le volume (virtualisation
  effective), spacer à 32 000 000px cohérent avec 1M lignes, scroll
  incrémental ~18ms/pas (sous le budget d'une frame 60fps), patch live
  toujours appliqué correctement à cette échelle.
- **Phase 6 — backend WASM** : `infrastructure/storage/opfs` (implémente
  `core.Storage` via `syscall/js` + sync access handles OPFS, pont
  async→sync par channel pour `getDirectory`/`getFileHandle`/
  `createSyncAccessHandle`), `infrastructure/wasmbridge` (miroir
  d'`infrastructure/ws` : mêmes fenêtres/coalescing, `postMessage` au lieu
  de WebSocket, exports set/get/delete/browse/query/stats + seed
  déclenché depuis le navigateur), `cmd/wasm` (composition root,
  `//go:build js && wasm`). Vérifié dans un vrai Worker Chromium
  (Playwright) : build Go→WASM chargé et exécuté, CRUD, seed de 500 clés
  en environ 0,1s, patch live filtré par fenêtre, et surtout **persistance
  OPFS confirmée à travers un redémarrage de Worker** (équivalent d'un
  reload d'onglet) — un `set` fait juste avant la fermeture du Worker est
  bien retrouvé après réouverture, à condition d'avoir laissé passer le
  cycle de flush (~1s, même compromis durabilité que le serveur natif).
- **Phase 7 — client WASM + bascule dans l'UI** : `web/src/client/wasmClient.ts`
  (même interface `GoRedisClient` que `serverClient.ts`, protocole
  requête/réponse corrélé par `requestId` par-dessus `postMessage`),
  sélecteur de backend dans `App.tsx`, `SeedPanel` (peuplement déclenché
  depuis le navigateur, visible seulement en WASM), `BenchmarkPanel`
  (SET/GET p50/p95, comparaison serveur vs WASM). Vérifié de bout en bout
  au navigateur (Playwright) : bascule serveur→WASM→serveur, seed,
  scroll infini et CRUD identiques sur les deux backends, benchmark
  affichant des latences WASM nettement plus basses (~0.1-0.3ms) que le
  serveur (~3-4ms REST), sans erreur console.

### Bugs trouvés et corrigés en vérifiant (pas en relisant)

- Double encodage du timestamp dans `BTree.Insert` (perte de précision
  float64 sur un unix-nano) → `escapeNumeric`.
- Snapshot sans timestamp par clé → toutes les clés restaurées au même
  instant → format de snapshot étendu (`core.SnapshotEntry`).
- `Timestamp int64` sérialisé en JSON number → perte de précision côté
  JS (> 2^53) → `json:",string"` + `BigInt` côté front.
- `useState(() => createServerClient())` → double WebSocket sous
  StrictMode (effet de bord dans un initialiseur) → pattern ref-guard.
- Mode "Activity" affichait le plus ancien en premier (le BTree ne
  parcourt qu'en ascendant) → `invertForDescending` sur l'encodage du
  timestamp dans `timeIndex`.
- À l'échelle 1M, un grand saut de scroll (glisser la scrollbar loin en
  avant) laissait un vide muet, sans retour visuel : la pagination par
  curseur ne charge que séquentiellement depuis le début (c'est le prix du
  O(log n) plutôt qu'un offset O(n), un choix délibéré) — mais rien
  n'indiquait à l'utilisateur que ça chargeait encore. Corrigé en affichant
  un placeholder "…" honnête pour tout index pas encore chargé, plutôt
  qu'un espace vide qui a l'air cassé. La limite en elle-même (pas de vrai
  accès aléatoire à une position arbitraire) reste assumée, pas "corrigée"
  au sens où on ne l'a pas fait disparaître — voir dette ci-dessous.
- `<InfiniteList key={backend+mode}>` se remontait bien à la bascule de
  backend, mais son `client` prop restait celui de l'ANCIEN backend pendant
  un rendu (bundle recréé un rendu plus tard, via `useEffect`) : la liste
  WASM chargeait des données du serveur pendant un instant. Corrigé en
  rendant la recréation du bundle synchrone avec le changement de
  `backend` (garde exécuté pendant le rendu, via ref), plutôt que dans un
  effet séparé.
- Une fois le store WASM vide au montage (avant tout seed), `hasMore`
  restait bloqué à `false` pour toujours : rien ne relançait le
  chargement une fois le seed terminé. Corrigé en réarmant `hasMore` dès
  que `totalCount` (stats) dépasse ce qui est chargé.
- **Interblocage réel entre le mutex du moteur et la boucle d'évènements
  JS** : `Snapshot()` gardait `e.mu` verrouillé pendant tout l'appel à
  `Storage`, y compris les opérations OPFS qui attendent une Promise JS
  (donc rendent la main à la boucle d'évènements). Un message entrant
  (ex. `browse`) tournant dans un callback JS synchrone et ayant lui aussi
  besoin d'`e.mu` s'y bloquait — et comme il ne rendait jamais la main, la
  boucle d'évènements ne pouvait plus jamais traiter la Promise censée
  débloquer `Snapshot()` : deadlock. Reproduit de façon déterministe
  (lancer un benchmark côté serveur puis un seed WASM juste après) et
  corrigé en relâchant `e.mu` avant les appels à `Storage` dans
  `Snapshot()`, sur le modèle de `Flush()` qui le faisait déjà
  correctement. C'est le bug le plus sérieux trouvé sur ce projet — un
  simple test séquentiel (seed puis browse l'un après l'autre) ne
  l'aurait jamais révélé, il fallait deux opérations concurrentes ayant
  toutes deux besoin du verrou pendant qu'une attente OPFS était en cours.

### Dette / points à surveiller (non bloquants, pas encore traités)

- `state_persistant.json`/`buffer_persistant.txt` locaux (gitignorés) sont
  dans l'ancien format, incompatibles avec `core.SnapshotEntry` — à
  supprimer avant le prochain `cmd/server` local (juste des données de
  test manuelles, rien d'important).
- `infrastructure/repl.Run` : `ReadString('\n')` sur un stdin fermé/non
  interactif (ex. lancé sans TTY sous un supervisor) boucle sans jamais
  bloquer — bug préexistant au refactor, pas encore corrigé. À traiter si
  `cmd/server` doit tourner sans REPL attaché (ex. Docker).
- Scroll infini = chargement séquentiel depuis le début (pagination par
  curseur). Un saut très loin en avant (scrollbar draguée à fond sur 1M
  lignes) affiche désormais un état de chargement honnête, mais rattraper
  concrètement des centaines de milliers d'entrées reste lent (des
  centaines de requêtes de page en série). Un vrai "aller à la position N"
  demanderait un skip côté serveur (O(log n + N) en mémoire, pas O(N) sur
  le réseau) — pas fait, jugé hors scope pour l'instant vu l'usage réel
  (scroll incrémental, pas de navigation par position).

## Reste à faire

### Phase 8 — Vérification finale double backend

Déjà couvert au fil de l'eau : bascule serveur↔WASM testée côte à côte
(Phase 7), persistance OPFS à travers un redémarrage de Worker (Phase 6),
compteurs de rendu par ligne visibles à l'écran (Phase 4, pas encore
revérifiés spécifiquement avec `wasmClient`). Reste :
- Documenter la commande de build WASM reproductible (README ou script) —
  la commande existe (`GOOS=js GOARCH=wasm go build -o web/public/goredis.wasm ./cmd/wasm`)
  mais n'est nulle part écrite pour quelqu'un d'autre que nous.
- Revérifier explicitement le rendu fin (une seule ligne re-render) avec
  `wasmClient` — fait avec `serverClient` en Phase 4, pas encore repris à
  l'identique côté WASM.
- Consigner des chiffres de benchmark propres (méthodo : machine,
  navigateur, taille de base, nb d'itérations) — le `BenchmarkPanel`
  existe et donne des résultats cohérents (WASM ~10-15x plus rapide que
  REST sur SET/GET, cf. Phase 7), mais rien n'est encore écrit dans un
  rapport.
