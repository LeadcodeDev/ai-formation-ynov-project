# Equipment loans API

Corrigé du TP : API Go de gestion d'emprunts de matériel. Besoin : `README.md`. Contrats, acceptance et partition : `docs/chantiers/reference-solution.md`. Décisions : `docs/adr/`. Vocabulaire : `docs/glossary.md`.

## Où vit quoi

- `internal/domain` : entités, erreurs, règles pures (`policy.go` : retard et `due_at` en Europe/Paris). N'importe rien d'infra.
- `internal/ports` : interfaces (`Transactor`, repositories, `Clock`, `IDGenerator`).
- `internal/service` : cas d'usage. L'autorisation vit ici, pas dans les handlers.
- `internal/adapters/{http,auth,postgres,mcp}` : un adaptateur par préoccupation externe. Les DTO restent dans l'adaptateur.
- `cmd/api` : racine de composition, seul endroit qui nomme les adaptateurs concrets.
- `migrations/` : SQL embarqué, appliqué au démarrage par `postgres.Migrate`.
- `deploy/` : compose et provisioning FerrisKey.

## Vérifier

- `make test` : tests Go via Docker (Go absent de l'hôte). Postgres de test : `TEST_DATABASE_URL`.
- `make up && make smoke` : stack complète et scénario AC-12.

## Règles

- Zéro commentaire dans le code. Le pourquoi va dans le commit ou un ADR.
- Une ligne Acceptance = un test dont le nom contient `AC-n`.
- Ne pas lancer `go get` ou `go mod tidy` pendant un travail parallèle.
