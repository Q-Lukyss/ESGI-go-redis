```
go run .
```

(ou build puis exécuter : `go build -o goredis.exe . && ./goredis.exe`)

Commandes dans le REPL : `SET <clé> "<valeur>"`, `GET <clé>`, `DELETE <clé>`, `GET WHERE value equals "x"`, `GET WHERE value contains "x"`, `GET WHERE value > 5`, batch avec `;`, `h` pour l'aide, `q` pour quitter.

## Lancer les tests

```bash
go test ./...
```

Avec détail :

```bash
go test ./engine/... -v
```
