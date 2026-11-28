#!/bin/sh
# make demo: from a fresh clone to a seeded, running Vaultly in one command.
# Needs only Docker. Safe to re-run: config is kept and seeding is idempotent.
set -eu
cd "$(dirname "$0")/.."

say() { printf '\033[36m==>\033[0m %s\n' "$*"; }

# 1. .env with a generated JWT secret (never overwrites an existing value).
if [ ! -f .env ]; then
  cp .env.example .env
  say "created .env from .env.example"
fi
if ! grep -q '^JWT_SECRET=..*' .env; then
  secret=$(openssl rand -hex 32 2>/dev/null || od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
  tmp=$(mktemp)
  sed "s/^JWT_SECRET=.*/JWT_SECRET=$secret/" .env > "$tmp" && mv "$tmp" .env
  say "generated JWT_SECRET"
fi

# 2. The rate limiter is built from its own repo. Use the sibling checkout if there is one,
#    otherwise let Docker fetch it from GitHub.
src=$(sed -n 's/^RATELIMITER_SRC=//p' .env | tail -1)
src=${RATELIMITER_SRC:-${src:-../Distributed-rate-limiter}}
case "$src" in
  http*|git@*) ;;
  *) if [ ! -d "$src" ]; then
       export RATELIMITER_SRC="https://github.com/saravana-devx/Distributed-rate-limiter.git#main"
       say "no $src checkout; building the rate limiter from GitHub"
     fi ;;
esac

# 3. Start everything and wait until the health checks pass.
say "starting the stack (first run builds images, a few minutes)"
docker compose up -d --build --wait

# 4. Seed demo data through the API.
say "seeding demo data"
out=$(docker compose exec -T backend server seed-demo)
token=$(printf '%s\n' "$out" | sed -n 's/^DEMO_SERVICE_TOKEN=//p')

cat <<EOF

  Vaultly is running.

  App        http://localhost:${APP_PORT:-3000}
  Admin      maya@demo.vaultly.dev / demo-password-2026   (workspace "Acme")
  Developer  sam@demo.vaultly.dev  / demo-password-2026   (has a change request waiting on prod)
  Mail       http://localhost:${MAIL_UI_PORT:-8025}

  Things to try
    - payments-api > Compare: drift between dev, staging and prod, without revealing values
    - payments-api > Changes: approve Sam's production change as Maya
    - Settings > Two-factor sign-in
EOF
if [ -n "$token" ]; then
  cat <<EOF

  CLI (read-only token for prod, shown once):
    export VAULTLY_URL=http://localhost:${API_PORT:-8080}
    docker compose exec -e VAULTLY_URL=http://127.0.0.1:8080 -e VAULTLY_TOKEN=$token backend vaultly pull

EOF
else
  echo
  echo "  (demo data already existed, so no new token was printed)"
  echo
fi
