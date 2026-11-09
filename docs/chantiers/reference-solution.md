# Chantier : solution de référence

Source de vérité du besoin : `README.md` (spec). Ce fichier fige les contrats, la partition des fichiers et l'état des chantiers.

## Décisions

- Quotas et durées : valeurs du §4 (quota 3, 7 jours portable ou matériel unitaire, 14 jours kit, 1 prolongation). La durée vient de `equipment_model.max_loan_days`.
- Index unique partiel ★ inclus dans la migration (le dépôt est un corrigé).
- IAM : FerrisKey, voir `docs/adr/0002-ferriskey-provisioning.md`.
- MCP : adaptateur dans le même hexagone, appelle `service.Service` directement, avec le token de l'utilisateur. SDK `github.com/modelcontextprotocol/go-sdk` v1.8.0.
- Règle de retard : `docs/adr/0001-overdue-rule.md`.
- Zéro commentaire dans le code (skill `architecture`).
- Go 1.25, module `github.com/ynov/equipment-loans`.

## Acceptance

Chaque ligne a un test. Les tests citent l'identifiant (`AC-n`) dans leur nom.

| ID | Ligne |
|---|---|
| AC-1 | Deux emprunts simultanés du même exemplaire : un seul réussit, les autres reçoivent `ErrItemNotBorrowable` (statut déjà `borrowed` après le verrou de ligne) ou `ErrItemAlreadyBorrowed` (index unique partiel en dernier recours). Testé au niveau service sur le vrai Postgres. |
| AC-2 | Un étudiant a 3 emprunts actifs maximum. Le 4e reçoit `ErrQuotaExceeded`. Des emprunts simultanés du même étudiant sur des exemplaires différents ne dépassent jamais 3 (`LockBorrower`). |
| AC-3 | `due_at` = minuit Europe/Paris clôturant le jour (date Paris de `borrowed_at` + `max_loan_days`). Tests table-driven sur les bornes (veille, jour dit, lendemain, changement d'heure). |
| AC-4 | Un étudiant avec au moins un emprunt en retard reçoit `ErrBorrowerHasOverdueLoan`. Le test combine retard et quota : le retard est signalé en premier. |
| AC-5 | Prolongation : une seule, jamais si en retard (`ErrLoanOverdue`), limite atteinte `ErrExtensionLimitReached`. `due_at` avance de `max_loan_days` jours calendaires Paris. Prolonger l'emprunt d'un autre renvoie `ErrLoanNotFound`. |
| AC-6 | Un exemplaire `maintenance` ou `lost` renvoie `ErrItemNotBorrowable`. Un exemplaire `borrowed` aussi. |
| AC-7 | Retour : `ok` remet l'exemplaire `available`, `damaged` le passe `maintenance`. Un second retour renvoie `ErrLoanAlreadyReturned`. |
| AC-8 | Un étudiant ne voit que ses emprunts (`MyLoans` filtre sur `actor.Sub`). Les actions staff renvoient `ErrForbidden` pour un étudiant. `OverdueLoans` n'est accessible qu'au staff et à l'admin. |
| AC-9 | JWT : signature via JWKS, `iss`, `aud`, expiration vérifiés. Token invalide ou absent : 401. Rôle insuffisant : 403. |
| AC-10 | `GET /healthz` public, 200 `{"status":"ok"}`. |
| AC-11 | Le serveur MCP expose `list_available_items`, `borrow_item`, `return_item`, `extend_loan`, `my_loans`, avec schémas d'entrée stricts, et agit avec l'identité du token de l'utilisateur. |
| AC-12 | `docker compose up` démarre Postgres, FerrisKey, le provisioning et l'API. Un token de test obtenu auprès de FerrisKey passe `GET /loans/me`. Test : `make up && make smoke`. |

Tout cas d'usage refuse (`ErrForbidden`) une identité sans rôle reconnu.

Ordre des contrôles de `BorrowItem` : acteur staff ou admin, emprunteur non vide, exemplaire existant (verrouillé), exemplaire `available`, verrou emprunteur, emprunteur sans retard, quota, insertion.

Autorisation des cas d'usage :

| Cas d'usage | Rôle minimum |
|---|---|
| `ListItems`, `MyLoans`, `ExtendLoan` | student (tout rôle authentifié) |
| `BorrowItem`, `ReturnLoan`, `OverdueLoans` | staff |

Erreurs HTTP : `ErrInvalidInput` 400, `ErrForbidden` 403, `ErrModelNotFound` `ErrItemNotFound` `ErrLoanNotFound` 404, les autres erreurs métier 409, erreur inconnue 500 (`internal`). Le champ `code` est le nom snake_case de l'erreur (voir `api/openapi.yaml`).

## Hors périmètre (MVP du README §6)

Déclarer un exemplaire en maintenance hors retour `damaged`, gérer catalogue et quotas, historique complet pour admin, réservations, notifications, audit. Les quotas sont des constantes de `domain`.

## Contrats gelés (lecture seule pour les sous-agents)

- `internal/domain/{identity,catalog,loan,errors}.go`
- `internal/ports/ports.go`
- `internal/service/commands.go` et les signatures de `internal/service/service.go`
- `api/openapi.yaml`
- Adaptateurs, signatures publiques attendues :
  - `internal/adapters/postgres` : `NewPool(ctx, dsn) (*pgxpool.Pool, error)`, `Migrate(ctx, pool) error`, `NewTransactor(pool) *Transactor` (implémente `ports.Transactor`).
  - `internal/adapters/auth` (package `auth`) : `Config{Issuer, Audience, JWKSURL, RolesClaim string}`, `NewVerifier(ctx, Config) (*Verifier, error)`, `(*Verifier).Verify(ctx, token string) (domain.Identity, error)`. Erreur d'authentification : `auth.ErrUnauthenticated`. `RolesClaim` accepte un chemin pointé (ex. `realm_access.roles`), défaut `roles`.
  - `internal/adapters/http` (package `httpapi`) : `NewHandler(svc Service, verifier Verifier, logger *slog.Logger) http.Handler` avec `Service` et `Verifier` définis comme interfaces dans le package.
  - `internal/adapters/mcp` (package `mcpserver`) : `NewHandler(svc Service, verifier Verifier, logger *slog.Logger) http.Handler` (transport streamable HTTP, token Bearer de l'utilisateur) avec `Service` et `Verifier` définis comme interfaces dans le package.

Variables d'environnement de l'API : `PORT` (8080), `DATABASE_URL`, `OIDC_ISSUER`, `OIDC_AUDIENCE`, `OIDC_JWKS_URL`, `ROLES_CLAIM`, `MCP_PORT` (8081, désactivé si vide).

## Partition des fichiers

| Chantier | Mission | Fichiers possédés |
|---|---|---|
| A | Règles métier et service | `internal/domain/policy*.go`, `internal/domain/policy*_test.go`, `internal/service/service.go`, `internal/service/*_test.go`, `internal/service/servicetest/**` |
| B | Postgres | `internal/adapters/postgres/**`, `migrations/**` |
| C | HTTP et JWT | `internal/adapters/http/**`, `internal/adapters/auth/**` |
| D | Infra | `Dockerfile`, `.dockerignore`, `Makefile`, `deploy/**`, `.github/workflows/**` |
| E | MCP | `internal/adapters/mcp/**` |

Possédés par l'orchestrateur : `go.mod`, `go.sum`, `cmd/api/**`, `api/openapi.yaml`, `internal/domain/{identity,catalog,loan,errors}.go`, `internal/ports/**`, `internal/service/commands.go`, `docs/**`, `scripts/**`, `README.md`, `CLAUDE.md`. Un sous-agent qui a besoin d'une modification là-dedans rapporte la ligne exacte.

Régénérés à l'intégration : `go.sum` (`scripts/go.sh go mod tidy`).

Environnement par chantier : B teste sur `TEST_DATABASE_URL=postgres://postgres:postgres@ynov-pg:5432/loans_b?sslmode=disable` (Postgres lancé par l'orchestrateur sur le réseau docker `ynov`). Les autres chantiers n'ont pas besoin de base. Aucun port fixe.

## Commandes

Go n'est pas installé sur l'hôte. Toute commande Go passe par `scripts/go.sh` (conteneur `golang:1.25-alpine`, réseau `ynov`) : `scripts/go.sh go test ./internal/service/...`. Ne jamais lancer `go mod tidy` ni `go get` : les dépendances sont déjà dans `go.mod`.

## État

| Chantier | État |
|---|---|
| 0 contrats | intégré |
| A | en cours |
| B | en cours |
| C | en cours |
| D | en cours |
| E | en cours |

Régime de tests : les sous-agents tournent en parallèle dans un checkout partagé, donc chacun ne lance que ses propres paquets. Seule l'exécution complète de l'orchestrateur après intégration compte.
