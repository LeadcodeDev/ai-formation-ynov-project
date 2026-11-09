# Projet TP : API de gestion d'emprunts de matériel (Go)

Cours « Outils IA », Ynov Bachelor 1. 12h : mardi 8h (4h théorie le matin + 4h TP l'après-midi), jeudi matin 4h.

## 0. Objectif du projet

Le projet sert de support pour montrer, avec l'IA générative comme outil de développement :

- ce que l'IA fait bien (squelette, boilerplate, tests de base) ;
- où elle se trompe (concurrence, règles métier combinées, autorisation, fuseaux horaires) ;
- pourquoi l'architecture, les contrats (OpenAPI) et la qualité d'ingénierie restent la responsabilité du développeur.

Jeudi, on ajoute un **serveur MCP** au-dessus du service pour qu'un LLM pilote l'application en langage naturel.

## 1. Contraintes imposées aux élèves

| Contrainte | Détail |
|---|---|
| Langage | Go |
| Base de données | **PostgreSQL obligatoire** |
| Authentification | **Obligatoire via un IAM** (FerrisKey ou Keycloak), validation du JWT via JWKS |
| Déploiement | **Dockerisé obligatoire** (Dockerfile multi-stage + docker-compose) |
| IA | Autorisée et encouragée (Copilot, Cursor, Claude), avec revue critique obligatoire |

## 2. Acteurs et rôles (portés par l'IAM)

| Rôle | Droits |
|---|---|
| `student` | Consulter le catalogue, voir **ses** emprunts, demander une prolongation |
| `staff` | Enregistrer un emprunt ou un retour pour un étudiant, déclarer un exemplaire en maintenance, lister les retards |
| `admin` | Tout ce que fait `staff`, plus gérer le catalogue et les quotas, voir tout l'historique |

L'identité de l'emprunteur est le claim `sub` du JWT. **Pas de table `users`** : les élèves n'ont pas à réimplémenter l'auth.

## 3. Modèle métier

- **Modèle de matériel** (`equipment_model`) : nom, catégorie, durée max de prêt. Ex. « Raspberry Pi 5 », « Casque VR ».
- **Exemplaire** (`item`) : numéro d'inventaire unique, référence au modèle, état `available | borrowed | maintenance | lost`.
- **Emprunt** (`loan`) : exemplaire, emprunteur (`sub`), date de sortie, date de retour prévue, date de retour effective, état au retour (`ok | damaged`), nombre de prolongations.

> Piège pédagogique : l'IA a tendance à tout mettre dans une seule table `equipment`. Distinguer modèle et exemplaire est volontaire.

### Esquisse de schéma SQL (à fournir aux élèves, sauf l'index marqué ★)

```sql
CREATE TABLE equipment_model (
  id            UUID PRIMARY KEY,
  name          TEXT NOT NULL,
  category      TEXT NOT NULL,
  max_loan_days INT  NOT NULL CHECK (max_loan_days > 0)
);

CREATE TABLE item (
  id               UUID PRIMARY KEY,
  model_id         UUID NOT NULL REFERENCES equipment_model(id),
  inventory_number TEXT NOT NULL UNIQUE,
  status           TEXT NOT NULL CHECK (status IN ('available','borrowed','maintenance','lost'))
);

CREATE TABLE loan (
  id              UUID PRIMARY KEY,
  item_id         UUID NOT NULL REFERENCES item(id),
  borrower_sub    TEXT NOT NULL,
  borrowed_at     TIMESTAMPTZ NOT NULL,
  due_at          TIMESTAMPTZ NOT NULL,
  returned_at     TIMESTAMPTZ,
  return_state    TEXT CHECK (return_state IN ('ok','damaged')),
  extensions_used INT NOT NULL DEFAULT 0
);

-- ★ À LAISSER EN POINT DE RÉFLEXION : garantit un seul emprunt actif par exemplaire
-- CREATE UNIQUE INDEX one_active_loan_per_item ON loan(item_id) WHERE returned_at IS NULL;
```

## 4. Règles métier (le cœur du TP)

1. Un exemplaire ne peut avoir qu'**un seul emprunt actif**.
2. **Quota** : 3 emprunts actifs maximum par étudiant.
3. **Durée max** selon le modèle/la catégorie (ex. 7 jours pour un portable, 14 pour un kit).
4. Un étudiant avec **au moins un emprunt en retard** ne peut plus emprunter.
5. **Prolongation** : une seule fois, et seulement si l'emprunt n'est pas déjà en retard.
6. Un exemplaire en `maintenance` ou `lost` n'est pas empruntable.
7. **Retour** : l'état est constaté (`ok | damaged`). Un exemplaire `damaged` passe automatiquement en `maintenance`.
8. Un étudiant ne voit que **ses** emprunts ; `staff` et `admin` voient tout.

Règle de retard à imposer explicitement : *un emprunt est en retard si `now` dépasse la date prévue, évaluée à minuit Europe/Paris*.

Valeurs de départ (à ajuster selon le contexte école, voir §11) : quota 3, durée 7 jours (portable/matériel unitaire) ou 14 jours (kit), 1 prolongation.

## 5. Pièges volontaires (supports de revue critique)

| Piège | Règle liée | Ce qu'on observe | Bonne réponse |
|---|---|---|---|
| **Concurrence** : deux requêtes simultanées sur le même exemplaire | 1 | L'IA écrit `check then insert`, qui marche en démo | Index unique partiel Postgres (★) ou transaction avec `SELECT ... FOR UPDATE` |
| **Calcul du retard** : fuseau, fin de journée, week-ends | 4, 5 | L'IA invente sa propre règle | Règle posée dans le contrat, tests table-driven sur les bornes |
| **Autorisation** : un étudiant devine l'ID d'un autre emprunt | 8 | Vérification faite seulement en middleware (ou pas du tout) | Contrôle dans le service + test dédié (accès à l'emprunt d'un autre → 403/404) |
| **Quota + retard combinés** | 2, 4 | L'IA implémente chaque règle isolément et oublie la combinaison | Tests combinant les deux conditions |
| **Bornes de dates** | 3, 5 | Erreur off-by-one sur la durée max | Tests table-driven |

## 6. Périmètre MVP (4h de TP)

| Endpoint | Rôle minimum | Description |
|---|---|---|
| `GET /items` (catalogue, filtre disponibilité) | student | Lister modèles/exemplaires |
| `POST /loans` | staff | Créer un emprunt pour un étudiant |
| `POST /loans/{id}/return` | staff | Enregistrer le retour + état |
| `POST /loans/{id}/extend` | student | Prolonger son emprunt |
| `GET /loans/me` | student | Mes emprunts |
| `GET /loans/overdue` | staff | Lister les retards |
| `GET /healthz` | public | Healthcheck pour Docker |

Hors MVP (bonus) : file d'attente de réservation, notifications de retard, CRUD complet du catalogue, historique d'audit.

## 7. Architecture cible (hexagonale légère)

```
.
├── cmd/api/main.go            # composition root
├── internal/
│   ├── domain/                # entités + règles métier pures (aucune dépendance)
│   ├── service/               # cas d'usage (BorrowItem, ReturnItem, ExtendLoan…)
│   ├── ports/                 # interfaces : LoanRepository, ItemRepository, Clock
│   └── adapters/
│       ├── http/              # handlers, middleware JWT, mapping erreurs → codes HTTP
│       ├── postgres/          # implémentation des repositories
│       └── mcp/               # (jeudi) serveur MCP
├── migrations/                # SQL fournis
├── api/openapi.yaml           # contrat (fourni)
├── deploy/
│   ├── docker-compose.yml     # api + postgres + IAM
│   └── realm/                 # realm pré-importé
├── Dockerfile
└── Makefile
```

Principes à faire constater :

- La logique métier vit dans `domain` et `service`, **pas** dans les handlers HTTP.
- `Clock` est une interface injectée, ce qui rend le retard testable sans `time.Sleep`.
- Les erreurs métier sont typées et mappées sur des codes HTTP dans l'adapter HTTP.

## 8. Infrastructure fournie aux élèves

`docker-compose.yml` avec :

- **PostgreSQL** (volume + migrations appliquées au démarrage) ;
- **IAM** : Keycloak ou FerrisKey, avec un **realm pré-importé** contenant :
  - un client OIDC pour l'API,
  - les rôles `student`, `staff`, `admin`,
  - 3 utilisateurs de test (un par rôle, ou deux étudiants + un staff + un admin pour tester l'isolation),
  - un mapper qui place les rôles dans le JWT ;
- l'**API** buildée via le Dockerfile.

Les élèves se limitent à valider le JWT (JWKS, `iss`, `aud`, expiration) et à mapper les rôles. La configuration d'un IAM reste une démo de l'intervenant.

`Dockerfile` : multi-stage (build Go → image `distroless` ou `alpine`), utilisateur non-root, `HEALTHCHECK` sur `/healthz`.

## 9. Déroulé mardi après-midi (4h)

| Créneau | Contenu |
|---|---|
| 0h00-0h30 | **Contrat d'abord** : lecture de `openapi.yaml`. Les élèves construisent leurs prompts à partir du contrat |
| 0h30-1h30 | **Architecture** : l'IA propose une structure, les élèves la critiquent et la comparent à la cible (§7) |
| 1h30-2h30 | **Règles métier + tests** table-driven (chevauchement, quota, retard, bornes). Mise en évidence des pièges (§5) |
| 2h30-3h15 | **Qualité** : `go vet`, `golangci-lint`, gestion d'erreurs, `slog`, validation des entrées |
| 3h15-4h00 | **Conteneurisation** : `docker compose up`, vérification du healthcheck, appel authentifié avec token |

Conseil : repo starter fourni (`go.mod`, `openapi.yaml`, squelette, docker-compose, migrations) pour ne pas perdre de temps au setup.

## 10. Jeudi matin (4h) : serveur MCP

- Le serveur MCP est un **adaptateur de plus** sur le même `service` (même hexagone), ou un client de l'API REST. Comparer les deux approches.
- SDK Go : `github.com/modelcontextprotocol/go-sdk` (officiel) ou `mark3labs/mcp-go`. **À vérifier avant le cours**, l'API évolue vite.
- Outils à exposer (schémas d'entrée stricts) : `list_available_items`, `borrow_item`, `return_item`, `extend_loan`, `my_loans`.
- La qualité des **descriptions d'outils** détermine l'usage par le LLM : bon sujet de prompting.
- Test avec Claude Desktop ou MCP Inspector.
- Exercice : « l'outil a une mauvaise description, que fait le modèle ? ».
- **Identité** : le LLM agit avec le **token de l'utilisateur** (ses droits, pas ceux d'un compte de service) pour illustrer la délégation d'identité (recommandation à confirmer, §11).
- Sécurité : actions destructrices ou sensibles avec confirmation, jamais de privilèges élargis via le LLM.

## 11. Décisions prises

1. **Quotas et durées** : valeurs du §4 conservées.
2. **Migrations Postgres** : fournies, index unique partiel ★ inclus (ce dépôt est le corrigé). Les données de démo sont dans une migration séparée.
3. **MCP et identité** : token de l'utilisateur.
4. **IAM de référence** : FerrisKey. Il n'a pas d'import de realm par fichier : le realm est provisionné par un script via son API (`deploy/realm`, voir `docs/adr/0002-ferriskey-provisioning.md`).

Autres décisions : règle de retard précisée dans `docs/adr/0001-overdue-rule.md`. Contrats, partition et état du chantier : `docs/chantiers/reference-solution.md`.

## 12. Bonus (élèves rapides)

1. Persistance avancée : migrations versionnées (golang-migrate).
2. Rôles plus fins et tests d'autorisation systématiques.
3. `docker-compose` incluant aussi le serveur MCP.
4. Tests de contrat : vérifier que l'API respecte l'OpenAPI.
5. Ressource MCP en lecture seule (catalogue, historique d'un étudiant).
6. Exercice **red team** : faire faire une bêtise au serveur MCP par prompt injection (emprunter 50 exemplaires, lire les emprunts d'un autre étudiant).
7. CI GitHub Actions : lint + tests + build de l'image.

## 13. Checklist de préparation (état de la solution de référence)

- [x] Trancher les décisions du §11
- [x] Écrire `api/openapi.yaml`
- [x] Écrire les migrations SQL (l'index ★ est inclus dans le corrigé ; à retirer de `migrations/0001_init.sql` pour le starter élèves)
- [x] Préparer `docker-compose.yml` (`deploy/`) + provisioning du realm
- [ ] Préparer le repo starter (à dériver de ce corrigé)
- [x] Réaliser le projet complet une fois (solution de référence)
- [x] Préparer les tests pièges (concurrence, bornes, autorisation, quota + retard)
- [x] Vérifier le SDK MCP Go (`modelcontextprotocol/go-sdk` v1.8.0) et le test via client MCP
- [ ] Tester avec Claude Desktop / MCP Inspector
- [ ] Tester le tout sur une machine « vierge » (Docker uniquement)

## 14. Démarrer, utiliser, arrêter

Prérequis : Docker. Go n'est pas requis sur l'hôte (`scripts/go.sh` exécute Go dans un conteneur).

### Démarrer

```
make up      # Postgres, FerrisKey, provisioning du realm, API (:8080) et serveur MCP (:8081)
make smoke   # scénario de bout en bout (AC-12), facultatif
```

Le premier `make up` construit les images et prend plus de temps. L'API applique les migrations au démarrage.

### Utiliser l'API REST

Utilisateurs de test (mot de passe dans `deploy/README.md`) : `student1`, `student2`, `staff`, `admin`.

```
TOKEN=$(make -s token USER=student1)
curl -H "Authorization: Bearer $TOKEN" localhost:8080/items?available=true
curl -H "Authorization: Bearer $TOKEN" localhost:8080/loans/me
```

Les tokens expirent au bout de 5 minutes : relancez `make token`. Le contrat est dans `api/openapi.yaml`.

### Utiliser le serveur MCP

Le serveur MCP (`http://localhost:8081/`, transport HTTP streamable) agit avec l'identité du token de l'utilisateur : ses droits, pas ceux d'un compte de service. Un étudiant ne peut donc pas appeler `borrow_item` ou `return_item`.

```
make mcp-add                    # enregistre le serveur "loans" dans Claude Code, en staff
MCP_USER=student1 make mcp-add  # ou en étudiant
```

Puis relancez Claude Code et vérifiez avec `/mcp` que `loans` est connecté. Demandez par exemple « liste le matériel disponible » ou « quels sont mes emprunts ? ».

Outils : `list_available_items`, `borrow_item` (staff), `return_item` (staff), `extend_loan`, `my_loans`. `borrow_item` et `return_item` modifient l'état : leur description demande une confirmation explicite à l'utilisateur.

Le token enregistré expire au bout de 5 minutes : relancez `make mcp-add` (depuis n'importe quel dossier avec `make -C <chemin du dépôt> mcp-add`). Un 401 ou « Authorization has invalid value » signifie un token expiré ou vide, ou une stack arrêtée.

### Arrêter

```
make down    # arrête la stack et supprime les volumes (données et realm)
claude mcp remove --scope user loans   # retire le serveur MCP de Claude Code
```

Après un `make down`, le prochain `make up` recrée le realm avec de nouvelles clés : tout ancien token devient invalide.

### Tests

```
make test    # go test via Docker
```

Les tests d'intégration Postgres lisent `TEST_DATABASE_URL` et échouent s'il est vide, sans skip silencieux. Exemple avec un Postgres de test :

```
docker network create ynov
docker run -d --name ynov-pg --network ynov -e POSTGRES_PASSWORD=postgres postgres:17-alpine
docker exec ynov-pg psql -U postgres -c "CREATE DATABASE loans_test"
TEST_DATABASE_URL='postgres://postgres:postgres@ynov-pg:5432/loans_test?sslmode=disable' make test
```
