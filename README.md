# Vaultly

Self-hosted secrets management: encrypted storage, environments and folders, versioning, access control, audit log, CLI, and a React dashboard.

**Stack:** Go + Gin · PostgreSQL · Redis · Kafka · OpenBao · React (Vite) · everything runs in Docker.

## Run

```bash
cp .env.example .env            # then fill MASTER_KEY and JWT_SECRET
#   MASTER_KEY=$(openssl rand -base64 32)   (optional once OpenBao is in use)   JWT_SECRET=$(openssl rand -hex 32)
docker compose up -d --build
```

| Service | URL |
|---|---|
| App | http://localhost:3000 |
| API | http://localhost:8080 |
| Mailpit (catches invite emails in dev) | http://localhost:8025 |
| OpenBao (key service; root token in `openbao-bootstrap:/boot/root-token`) | http://localhost:8200 |

Run the tests in containers too:

```bash
docker run --rm --network vaultly_default \
  -e DATABASE_URL="postgres://vaultly:vaultly@postgres:5432/vaultly?sslmode=disable" -e REDIS_URL=redis://redis:6379/0 \
  -v "$PWD/backend":/src:ro -v vaultly-gomod:/go/pkg -w /src golang:1.26-alpine go test ./...
```

## Deploy (HTTPS on a single server)

```bash
cp deploy/env.prod.example .env      # fill in DOMAIN and the three generated secrets
docker compose -f docker-compose.prod.yml up -d --build
sh deploy/backup.sh                  # back up the database AND the OpenBao key service
```

Caddy gets a Let's Encrypt certificate automatically and only ports 80/443 are public. Full steps, checks and rollback: `docs/14_GO_LIVE_RUNBOOK.txt`. Plan for the next 2 days and 1 month: `docs/15_PLAN_2_DAYS_AND_1_MONTH.txt`.

## Features

**Core** — register/login (JWT + rotating refresh cookie), workspaces, projects, environments, folders (paths), encrypted secrets, versions and rollback, rotation, `.env` import/export, service tokens, audit log (via Kafka), CLI, dashboard.

**Security** — sliding-window rate limiting (Redis/Lua) · secret **expiry** (expired secrets stop being served and raise `secret.expired`) · service-token **IP allowlists** (denials are audited) · bcrypt, hashed tokens, single-use rotating refresh tokens.

**Teams** — **email invites** (SMTP, link also shown to the admin) · **per-environment roles** (`none` / `viewer` / `developer`, overriding the workspace role per environment name) · **personal access tokens** (act as the user, same roles, cannot mint more tokens).

**Secrets** — **references**: `${KEY}`, `${/path/KEY}`, `${env:/path/KEY}` resolved on read (cycle-safe; service tokens can only reference inside their own environment and path scope) · **tags** · **search** across paths and environments (never leaks environments you can't read).

**Integrations** — **webhooks** (HMAC-SHA256 signed, retried, private addresses blocked, never contain secret values) · **sync** to GitHub Actions secrets, Vercel and Railway (optional auto-sync on change) · **dynamic secrets**: short-lived PostgreSQL credentials, revoked and dropped automatically at expiry (and when the project or environment is deleted).

**Ops** — `/healthz` · OpenBao key service + key rotation · encrypted **backup & restore** (passphrase-sealed with scrypt + AES-256-GCM).

## Security model

The root key lives in **OpenBao** (Transit engine) and wraps a random per-project data key; the data key encrypts each value with AES-256-GCM, bound to the project ID. Only wrapped keys and ciphertext reach the database. `internal/crypto.KeyProvider` has a Local (`MASTER_KEY`) and an OpenBao implementation; `server rewrap` moves keys to the current OpenBao key version. Emails, names and audit actors are encrypted too, with an HMAC blind index so login still works — see `docs/13_OPENBAO_AND_PII_ENCRYPTION.txt`. **Back up the OpenBao volumes; losing them makes every secret unreadable.**

## CLI

```bash
export VAULTLY_TOKEN=vst_...  VAULTLY_URL=http://localhost:8080
vaultly pull > .env                 # or: --format shell|json
vaultly run -- node server.js       # inject secrets as env vars
vaultly set KEY value               # write-enabled token
curl -XPOST -H "Authorization: Bearer $VAULTLY_TOKEN" $VAULTLY_URL/api/v1/dynamic/readonly-db/lease
```

## Notes and limits

- **Backups** contain ciphertext and wrapped keys, so restoring needs the same `MASTER_KEY`. Tokens, webhooks and integration credentials are deliberately excluded.
- **Integrations** push additions and updates only; deleting a secret in Vaultly does not delete it at the destination. The provider adapters are tested against mock servers, not the live GitHub/Vercel/Railway APIs.
- **Dynamic secrets** support PostgreSQL only. `DROP OWNED` runs in the admin URL's database.
- **Webhooks**: `WEBHOOK_ALLOW_PRIVATE=true` is set in `docker-compose.yml` for local testing — leave it unset in production.
- Behind a reverse proxy, forwarded client IPs are trusted from private ranges only (`SetTrustedProxies` in `internal/bootstrap/app.go`). IP allowlists and rate limits depend on this.
- Not built: production-hardened OpenBao (the demo unseals with one key on a Docker volume), multi-region/HA (infrastructure concern — the API is stateless, so run replicas behind a load balancer with managed Postgres/Redis/Kafka), SSO, E2EE, PAM, certificate management.
