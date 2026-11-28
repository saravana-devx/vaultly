# Vaultly

[![CI](https://github.com/saravana-devx/vaultly/actions/workflows/ci.yml/badge.svg)](https://github.com/saravana-devx/vaultly/actions/workflows/ci.yml)

Self-hosted secrets management: encrypted storage, environments and folders, versioning, access control, audit log, CLI, and a React dashboard.

**Stack:** Go + Gin · PostgreSQL · Redis · Kafka · OpenBao · React (Vite) · everything runs in Docker.

**How it's built and why:** [docs/DESIGN.md](docs/DESIGN.md) covers the architecture, the threat model, the encryption design and its trade-offs.

## Try it in one command

```bash
make demo        # only needs Docker: creates .env, starts the stack, seeds a demo workspace
```

Then open http://localhost:3000 and sign in as `lily@demo.vaultly.dev` (admin) or `rose@demo.vaultly.dev` (developer), password `demo-password-2026`. Rose has a production change waiting for Lily's approval, and the **Compare** tab shows drift between environments.

## Run

```bash
cp .env.example .env            # then set JWT_SECRET=$(openssl rand -hex 32)
make up                         # = docker compose up -d --build   (make down to stop, make help for more)
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

**Core** — register/login (JWT + rotating refresh cookie) with optional **two-factor sign-in** (TOTP, single-use recovery codes, replay protection), workspaces, projects, environments, folders (paths), encrypted secrets, versions and rollback, rotation, `.env` import/export, service tokens, audit log (via Kafka), CLI, dashboard.

**Security** — sliding-window rate limiting (Redis/Lua) · secret **expiry** (expired secrets stop being served and raise `secret.expired`) · service-token **IP allowlists** (denials are audited) · bcrypt, hashed tokens, single-use rotating refresh tokens.

**Review** — **protected environments**: a developer's change becomes a **change request** that someone else must approve (stale requests are refused, never overwrite newer values) · **compare environments**: missing keys and differing values across dev/staging/prod, without revealing any value.

**Teams** — **email invites** (SMTP, link also shown to the admin) · **per-environment roles** (`none` / `viewer` / `developer`, overriding the workspace role per environment name) · **personal access tokens** (act as the user, same roles, cannot mint more tokens).

**Secrets** — **references**: `${KEY}`, `${/path/KEY}`, `${env:/path/KEY}` resolved on read (cycle-safe; service tokens can only reference inside their own environment and path scope) · **tags** · **search** across paths and environments (never leaks environments you can't read).

**Integrations** — **webhooks** (HMAC-SHA256 signed, retried, private addresses blocked, never contain secret values) · **sync** to GitHub Actions secrets, Vercel and Railway (optional auto-sync on change) · **dynamic secrets**: short-lived PostgreSQL credentials, revoked and dropped automatically at expiry (and when the project or environment is deleted).

**Ops** — `/healthz` · OpenBao key service + key rotation · encrypted **backup & restore** (passphrase-sealed with scrypt + AES-256-GCM).

## Security model

The root key lives in **OpenBao** (Transit engine) and wraps a random per-project data key; the data key encrypts each value with AES-256-GCM, bound to the project ID. Only wrapped keys and ciphertext reach the database. `internal/crypto.KeyProvider` has a Local (`MASTER_KEY`) and an OpenBao implementation; `server rewrap` moves keys to the current OpenBao key version. Emails, names and audit actors are encrypted too, with an HMAC blind index so login still works — see `docs/13_OPENBAO_AND_PII_ENCRYPTION.txt`. **Back up the OpenBao volumes; losing them makes every secret unreadable.** Secret values are encrypted by Vaultly itself; OpenBao only protects the keys, and `KEY_PROVIDER=local` runs without it (see `docs/13_OPENBAO_AND_PII_ENCRYPTION.txt`).

## CLI

```bash
vaultly login --url http://localhost:8080   # paste a service token (hidden); stored 0600 in ~/.config/vaultly
vaultly setup --path /app                   # link this folder via .vaultly.json (no secrets, safe to commit)
vaultly run -- node server.js               # inject secrets as env vars
vaultly pull > .env                         # or: --format shell|json
vaultly set KEY value                       # write-enabled token
vaultly whoami                              # token scope and where settings came from
```

In CI, set `VAULTLY_TOKEN` and `VAULTLY_URL` instead of logging in. If the server is unreachable, `run`/`pull` use an encrypted copy of the last fetch (key derived from the token, 24h max, never after a 401/403). Details in [docs/DESIGN.md](docs/DESIGN.md#10-cli-and-the-offline-cache).

## Notes and limits

- **Backups** contain ciphertext and wrapped keys, so restoring needs the same `MASTER_KEY`. Tokens, webhooks and integration credentials are deliberately excluded.
- **Integrations** push additions and updates only; deleting a secret in Vaultly does not delete it at the destination. The provider adapters are tested against mock servers, not the live GitHub/Vercel/Railway APIs.
- **Dynamic secrets** support PostgreSQL only. `DROP OWNED` runs in the admin URL's database.
- **Webhooks**: `WEBHOOK_ALLOW_PRIVATE=true` is set in `docker-compose.yml` for local testing — leave it unset in production.
- Behind a reverse proxy, forwarded client IPs are trusted from private ranges only (`SetTrustedProxies` in `internal/bootstrap/app.go`). IP allowlists and rate limits depend on this.
- Not built: production-hardened OpenBao (the demo unseals with one key on a Docker volume), multi-region/HA (infrastructure concern — the API is stateless, so run replicas behind a load balancer with managed Postgres/Redis/Kafka), SSO, E2EE, PAM, certificate management.
