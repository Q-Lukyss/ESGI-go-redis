# Roadmap pas-à-pas — Le moteur « WasmRedis » (Go)

Guide pour construire le **moteur Redis en Go**, étape par étape, sans se noyer. On se concentre **uniquement sur le moteur Go** : commandes, persistance, index, TTL.

> Volontairement, ce guide ne donne **pas le code** : juste des **indications** sur ce qu'il faut construire et la logique à l'intérieur. La conception (noms, types, signatures) et l'implémentation sont à vous — c'est là que vous apprenez Go.

---

## Comprendre Redis (à lire avant tout le reste)

Avant la moindre ligne de code, il faut le **modèle mental**. Tout le projet découle du schéma du tableau. Cette section le déroule, élément par élément.

### C'est quoi Redis ?

Redis est une base de données **clé-valeur** qui vit **en mémoire (RAM)**. On y range des paires `clé → valeur` (par ex. `name → "matt"`) et on les lit/écrit avec des commandes texte. C'est **ultra-rapide parce que tout est en RAM** : pas d'aller-retour disque à chaque requête.

### Le problème (et pourquoi tout le schéma existe)

La RAM est **volatile** : si le programme s'arrête (crash, fermeture d'onglet), **tout est perdu**. C'est la ligne **« RAM | D.D »** au milieu du tableau (D.D = disque dur) :
- à **gauche, la RAM** : rapide mais éphémère ;
- à **droite, le disque** : lent mais durable.

Tout l'enjeu de Redis — et de ce projet — c'est de **garder la vitesse de la RAM tout en survivant à un redémarrage**, en recopiant intelligemment les données vers le disque. Redis fait ça avec **deux mécanismes** de persistance, tous les deux dessinés sur le tableau.

### Le flux, élément par élément du schéma

1. **Le client envoie une commande** (le bonhomme à gauche) : `SET name "matt"`, `DELETE`, `GET`. La commande est une **string** ; `arg1` = la clé (`name`), `arg2` = la valeur (`"matt"`).

2. **Le `state` (en RAM)** — `const state = {}` puis `state[arg1] = arg2`. C'est le **store vivant** : la vraie donnée, celle qu'on lit en priorité. Une écriture met à jour `state` immédiatement.

3. **Le `buffer` (en RAM)** — `const buffer = []` puis `buffer.push(req)`. À chaque écriture, on **note aussi l'opération dans une file**. On n'écrit pas sur le disque à chaque `SET` (trop lent) : on **accumule** dans le buffer.

4. **Le flush toutes les ~1 s** — la boucle « 1 sec → trigger → empty buffer ». Une fois par seconde, on **vide le buffer** vers le disque. Le « 1 max à la fois » = jamais deux flush en parallèle. C'est le compromis vitesse/durabilité : au pire on perd **1 seconde** de données en cas de crash.

5. **Le « Buffer Persistant » sur disque = l'AOF (Append Only File)** — la boîte en haut à droite, avec les lignes `Set name "matt"`. Le disque reçoit **en append** (ajout au bout du fichier) chaque paquet d'opérations flushé. C'est un **journal** : la liste de tout ce qui s'est passé, dans l'ordre. « multi anti-collision » = un seul écrivain à la fois, pour ne pas corrompre le fichier.

6. **Le snapshot toutes les ~2 min = le « State Persistant » (RDB)** — la boîte en bas à droite, `file.json { name: "matt" }`. Toutes les 2 minutes, on écrit **l'état complet** dans un fichier : une **photo**. Puis on **vide l'AOF** (« empty buffer → update ») : puisque la photo contient déjà tout, le journal des opérations passées ne sert plus → c'est la **compaction**.

7. **Le restore au démarrage** (la flèche verte « restore ② », le « au démarrage » du tableau) — quand le moteur redémarre, il reconstruit la RAM depuis le disque : on **applique l'AOF par-dessus le snapshot** (les opérations arrivées après la dernière photo), puis on **remet le snapshot ainsi reconstitué en RAM**. Résultat : l'état est **identique** à avant l'arrêt.

8. **« nécessite B-Tree »** — c'est l'autre annotation, séparée : les `GET` filtrés ont besoin de **structures d'index** pour ne pas scanner toute la base à chaque requête. Selon l'opérateur : **equals** → un **index inversé** (`valeur → clés`, accès direct) ; **range** (`>`, `≥`, `<`, `≤`) → un **B-Tree** (index ordonné) ; **contains** → scan. (codé tout à la fin.)

### Le résumé en une image

```
            CLIENT
       "SET name matt"
              │
              ▼
   ┌────────────────────┐          RAM (rapide, volatile)
   │   state  {}        │◄── lecture (GET)
   │   buffer []        │──  push à chaque écriture
   └─────────┬──────────┘
             │ flush toutes les ~1s
═════════════╪═══════════════════  RAM │ DISQUE  ════════════
             ▼
   ┌────────────────────┐  append  ┌──────────────────────┐
   │  AOF (journal)     │◄──────────│ Set name "matt"      │
   └────────────────────┘          │ Set age 30 ...       │
             ▲                      └──────────────────────┘
             │ toutes les ~2 min : snapshot complet, puis on vide l'AOF
   ┌────────────────────┐
   │   snapshot.json    │  { "name": "matt" }  ← photo complète de l'état
   └────────────────────┘
             │
             │ au démarrage (restore) : appliquer l'AOF par-dessus le snapshot…
             └──────────────►  …puis remettre le résultat en RAM (state reconstitué)
```

### Vrai Redis vs notre version

Le schéma est une version **simplifiée et idéalisée** de Redis. Le vrai Redis utilise un format binaire pour le snapshot (RDB), un `fsync` configurable pour l'AOF, une réécriture (rewrite) de l'AOF, etc. Mais **le modèle mental est exactement celui-ci** : un store en RAM, un journal append-only, des snapshots périodiques, un restore qui combine les deux. C'est tout ce qu'il faut comprendre pour ce projet.

---

## Comment utiliser cette roadmap

- **Une phase = quelque chose qui tourne et qu'on peut tester.** On ne passe à la suite que quand le ✅ « fini quand » est vert.
- **Tout se fait en Go natif**, testable avec `go test`. Aucun navigateur dans ce document.
- **Ordre** : 1) le cœur SET/GET/DELETE, 2) **la persistance et le recovery** (le plus important), 3) le batch, 4) le **GET filtré + B-Tree** et le **TTL** (à la fin).
- **On préfère des méthodes sur le moteur** à des fonctions libres dès que ça touche l'état du moteur.
- Difficulté : 🟢 facile · 🟡 moyen · 🔴 piège classique.

---

## Phase 0 — Go : juste ce qu'il faut 🟢

> But : être à l'aise avec les briques Go qu'on va utiliser. Ici on montre la **syntaxe** du langage sur des exemples neutres.

### 0.1 — Installation & projet
```bash
go version                 # vérifier que Go est installé
mkdir wasmredis && cd wasmredis
go mod init github.com/vous/wasmredis
go get github.com/samber/lo
```

### 0.2 — Les briques à connaître (révisez-les, exemples neutres)
- **struct** : regrouper des champs sous un type.
- **map** `map[string]T` : associer une clé à une valeur ; la lecture renvoie aussi un booléen de présence (`v, ok := m["k"]`).
- **slice** `[]T` : liste dynamique ; on y ajoute avec `append`.
- **méthode** : une fonction rattachée à un type via un *receiver* (`func (c *Counter) Inc()`). C'est le pattern qu'on utilisera partout sur le moteur.
- **erreur** : Go renvoie l'erreur en dernière valeur de retour ; on la teste avec `if err != nil`.
- **interface** : un contrat de méthodes ; un type qui implémente ces méthodes « satisfait » l'interface.
- **samber/lo** : `lo.Map`, `lo.Filter`, `lo.Reduce`… à préférer aux boucles `for` (style fonctionnel attendu).

### 0.3 — Exercice de chauffe
Un programme qui lit des lignes au clavier (`bufio.Scanner` sur `os.Stdin`) et appelle des méthodes set/get/delete sur une petite struct contenant une map. But : parler Go, pas faire propre.

✅ **Fini quand** : l'exo tourne et un premier `*_test.go` passe au `go test ./...`.

---

## Phase 1 — Le cœur du moteur en Go natif 🟡

> But : SET / GET / DELETE qui marchent, testés. **Pas de filtres ici** (tout à la fin).

### 1.1 — Le parser de commandes 🟡
- Construire une **fonction** de parsing (pas une méthode : elle ne touche pas à l'état du moteur, c'est juste de la transformation de texte) qui transforme une string en une représentation structurée d'une commande.
- Concevoir vous-mêmes le **type** qui représente une commande parsée (son type de commande, et selon le cas sa clé / sa valeur).
- Logique interne : nettoyer et découper l'input ; identifier la commande (SET/GET/DELETE, insensible à la casse) ; extraire la clé, et pour SET la valeur (retirer les guillemets) ; renvoyer une **erreur claire** si la commande est inconnue ou s'il manque des arguments.
- 🔴 piège : une valeur contenant des espaces (`SET msg "hello world"`) casse un découpage naïf sur les espaces — à gérer, ou à assumer comme limite connue au début.
- Tests : un test par forme de commande + par cas d'erreur (rappel : `go test`, fonctions `TestXxx(t *testing.T)`).

✅ Fini quand : le parsing gère les 3 commandes + les erreurs, tests verts.

### 1.2 — Le moteur (tout en méthodes) 🟢
- Définir le type **moteur** qui contient le state (la map clé→valeur). On lui ajoutera d'autres champs en Phase 2.
- Un **constructeur** qui renvoie un moteur prêt à l'emploi (state vide).
- Des **méthodes** sur le moteur : poser une valeur ; lire une valeur (erreur si la clé est absente) ; supprimer une clé.
- Une **méthode d'aiguillage** qui prend une commande parsée et appelle la bonne méthode selon son type.

### 1.3 — Bout à bout 🟢
- Une **méthode** qui prend une string brute, la parse, puis l'exécute (en propageant l'erreur de parsing).
- Tests d'intégration : SET puis GET renvoie la valeur ; DELETE puis GET renvoie l'erreur « absente ».

✅ **Fin de Phase 1** : un mini-Redis en mémoire, piloté par des strings, entièrement testé.

---

## Phase 2 — Persistance & recovery (LA priorité) 🔴

> But : que les données **survivent à un redémarrage**. On reste en Go natif (fichiers sur le disque).

### 2.1 — L'interface `Storage` (décision la plus importante) 🔴
- Définir une **interface** `Storage` qui découple le moteur du support physique (le moteur ne doit pas savoir s'il écrit dans tel ou tel type de fichier). Elle doit permettre de : ajouter au journal (AOF), lire le journal, vider le journal, écrire le snapshot, lire le snapshot. À vous de nommer/typer les méthodes.
- 🔴 C'est CE découpage qui permettra de **changer de support de stockage plus tard sans toucher au moteur** : on changera juste d'implémentation. Ne sautez pas cette étape.

### 2.2 — Implémentation native (fichiers OS) 🟢
- Écrire une **implémentation** de `Storage` basée sur des fichiers du disque : un fichier pour l'AOF (ouvert en ajout), un fichier pour le snapshot.
- Indices : ouverture en append/create pour l'AOF ; simple lecture/écriture de fichier pour le reste ; bien fermer les fichiers.

### 2.3 — Le buffer d'écritures 🟡
- Concevoir le type qui représente une **opération** d'écriture (assez d'info pour la rejouer : son type, sa clé, sa valeur).
- Ajouter au moteur : une **file** d'opérations en attente, une référence au `Storage`, un **verrou**.
- Une méthode (privée) qui **enregistre** une opération dans la file, sous verrou — à appeler depuis les méthodes d'écriture.

### 2.4 — Le flush toutes les ~1 s 🟡
- Une méthode qui, **sous verrou**, récupère puis vide la file, sérialise les opérations et les ajoute à l'AOF via `Storage`. Ne rien faire si la file est vide.
- La déclencher ~1×/s (ticker dans une goroutine). Le verrou garantit « 1 flush à la fois ».

### 2.5 — Le snapshot toutes les ~2 min + compaction 🟡
- Une méthode qui sérialise **tout le state**, l'écrit comme snapshot via `Storage`, puis **vide l'AOF** (les opérations passées sont désormais couvertes par la photo).
- La déclencher ~toutes les 2 min.

### 2.6 — Le restore au démarrage 🔴
- Une méthode qui reconstitue le state : **appliquer l'AOF par-dessus le snapshot, puis remettre le résultat en RAM**. Concrètement : lire le snapshot et le charger dans le state ; puis lire l'AOF et **rejouer chaque opération** sur le state — **sans la re-logguer** dans le buffer.
- (plus tard) reconstruire aussi le B-Tree.

### 2.7 — Le test en or 🟢
- Un test qui : crée un moteur sur un **dossier temporaire**, écrit puis flush ; crée un **nouveau** moteur sur le **même** dossier et appelle restore ; vérifie que la donnée est bien là.
- Indice : `t.TempDir()`.

✅ **Fin de Phase 2 — checkpoint majeur** : on remplit la base, on « tue » le moteur, on le relance → l'état est identique. C'est le cœur du projet.

---

## Phase 3 — Batch 🟢

> But : exécuter plusieurs commandes en un seul appel (utile plus tard pour amortir l'aller-retour worker).

- Une **méthode** qui prend une liste de commandes, les exécute dans l'ordre et renvoie la liste des résultats. Style fonctionnel (`lo.Map`).
- Concevoir un type **résultat** (la valeur, et l'erreur éventuelle).

✅ Fini quand : une liste de commandes renvoie une liste de résultats alignée.

---

## Phase 4 — GET filtré + index (à la fin) 🔴

> But : les requêtes filtrées, chacune avec la structure d'index adaptée.
> Récap des index : **equals → index inversé** (O(1)) · **range → B-Tree** (ordonné) · **contains → scan**.

### 4.1 — equals : index inversé (key-value inversé) 🟡
- Construire un **index inversé** : au lieu de `clé → valeur`, une map `valeur → ensemble de clés` portant cette valeur (plusieurs entrées peuvent partager une valeur, ex. plusieurs `age=30`).
- Le **maintenir à jour** à chaque écriture/suppression (et le reconstruire au restore).
- `equals` devient alors un **accès direct O(1)** : on lit l'ensemble des clés pour la valeur cherchée, au lieu de scanner toute la base.

### 4.2 — contains : scan 🟢
- `contains` ne rentre pas dans un index simple (recherche de sous-chaîne) → parcours filtré (`lo.Filter` + `strings.Contains`).
- (bonus : index n-gram / trie pour accélérer — hors périmètre de base.)

### 4.3 — Range queries : index trié (tremplin) 🟡
- Faire marcher `>`, `>=`, `<`, `<=` avec une **slice triée + recherche dichotomique** (`sort.Search`).
- Objectif : que ça **marche**, avec des tests. Ce n'est pas l'archi finale.

### 4.4 — Le B-Tree maison 🔴
- Remplacer l'index trié par un **B-Tree** (ou B+Tree), maintenu à jour à chaque écriture (méthodes sur la structure d'index).
- **Réutiliser les mêmes tests** que 4.3 comme filet de sécurité.

> 🔴 N'écrivez pas le B-Tree parfait du premier coup : index trié qui marche → tests → B-Tree → mêmes tests.

✅ Fini quand : les filtres (equals via index inversé, range via B-Tree, contains via scan) renvoient les bons résultats, tests verts.

---

## Phase 5 — TTL / expiration (à la fin) 🟡

> But : des clés qui expirent.

### 5.1 — Une valeur avec échéance
- Faire évoluer le state pour qu'une entrée puisse porter une **date d'expiration optionnelle**.
- Parser une commande du type `SET name "matt" EX 60` (durée en secondes).

### 5.2 — Expiration lazy (à la lecture)
- À la lecture, si l'entrée est **expirée** : la supprimer et la traiter comme absente.

### 5.3 — Expiration active (balayage)
- Une **goroutine périodique** qui supprime réellement les clés expirées (state + index + persistance).
- **Tester avec une horloge injectable** (ne pas dépendre du vrai temps dans les tests).

✅ Fini quand : une clé avec TTL disparaît à l'échéance, en lazy ET en actif.

---

## Et après ?

Le moteur Redis en Go est complet : commandes, persistance, recovery, index, TTL. La suite du projet (portage, interface, etc.) viendra **plus tard, dans un second temps** — pour l'instant on se concentre uniquement sur ce moteur Go solide et bien testé.

---

## Organisation d'équipe

- **Phases 0 à 2 ensemble** : tout le monde apprend Go et comprend le moteur + la persistance. C'est le socle commun, ne le sautez pas.
- Ensuite on peut répartir : un binôme sur les filtres/index/B-Tree, un sur le TTL, un sur le batch et le benchmark.
- **Règle anti-blocage** : si une étape prend > 1 journée sans rien qui tourne, elle est mal découpée → version « moche mais qui marche », commit, puis améliorer.
