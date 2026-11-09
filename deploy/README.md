# Deploy

`docker compose -p ynov-loans -f deploy/docker-compose.yml up -d --build --wait` (or `make up`) starts:

| Service | Role | Port |
|---|---|---|
| `loans-db` | Postgres of the API | internal |
| `ferriskey-db`, `ferriskey-migrations`, `ferriskey` | FerrisKey IAM | 3333 |
| `realm-init` | one-shot, provisions realm `loans` through the FerrisKey admin API (idempotent) | none |
| `api` | the loans API | 8080 (HTTP), 8081 (MCP) |

Defaults live in `deploy/.env.example` (copy to `deploy/.env` to override). FerrisKey console admin: `admin` / `admin` on realm `master`.

## Test users

Realm `loans`, client `loans-api` (public, direct grant). All users share `TEST_USER_PASSWORD` (default `Loans-Test-2026!`; FerrisKey requires 12+ characters, an uppercase letter and a special character).

| Username | Role |
|---|---|
| `student1` | student |
| `student2` | student |
| `staff1` | staff |
| `admin1` | admin |

```
make token USER=staff1
```

or

```
curl -s -X POST http://localhost:3333/realms/loans/protocol/openid-connect/token \
  -d grant_type=password -d client_id=loans-api \
  -d username=student1 --data-urlencode 'password=Loans-Test-2026!'
```

Use `.access_token` from the response (valid 300 s):

```
curl -H "Authorization: Bearer $(make token USER=student1)" http://localhost:8080/loans/me
```

Always request tokens through `localhost:3333`, see below.

## Token claims (observed)

```
{"sub":"<uuid>","iss":"http://localhost:3333/realms/loans","aud":["loans-realm","account","loans-api"],
 "azp":"loans-api","typ":"Bearer","scope":"openid email loans-api-audience organization profile roles",
 "realm_access":{"roles":["student"]},"preferred_username":"student1","exp":...}
```

- Roles: `realm_access.roles` (native `roles` scope, mapper `oidc-usermodel-realm-role-mapper`). Use `ROLES_CLAIM=realm_access.roles`.
- Audience: `loans-api` is added by the scope `loans-api-audience` (`oidc-audience-mapper`) attached as default scope to the client. Use `OIDC_AUDIENCE=loans-api`.
- JWKS: `http://localhost:3333/realms/loans/protocol/openid-connect/jwks.json`, key type RS256.

## Issuer and hostnames

FerrisKey builds `iss` from the `Host` header of the request. A token obtained from the host through `localhost:3333` carries `iss=http://localhost:3333/realms/loans`. The API container therefore uses:

- `OIDC_ISSUER=http://localhost:3333/realms/loans` (compared to the claim, never fetched)
- `OIDC_JWKS_URL=http://ferriskey:3333/realms/loans/protocol/openid-connect/jwks.json` (fetched from inside the compose network)

A token obtained from inside the network (`http://ferriskey:3333`) has `iss=http://ferriskey:3333/realms/loans` and is rejected by this configuration.

## Provisioning

`deploy/realm/provision.sh` calls the FerrisKey admin API: admin token (`admin-cli`, password grant), realm, client, audience scope and mapper, realm roles, users, passwords, role assignments. Each step checks existence first, because FerrisKey answers 409, 400 or 500 on duplicates depending on the resource. Re-run it with `docker compose -p ynov-loans -f deploy/docker-compose.yml run --rm realm-init`.
