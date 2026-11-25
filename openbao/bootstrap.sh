#!/bin/sh
# Bootstrap for the local OpenBao used by Vaultly. It runs forever and, as needed:
#   1. initialises OpenBao the first time (1 key share - a DEVELOPMENT convenience)
#   2. unseals it after every restart
#   3. enables the Transit engine and creates the (non-exportable) key "vaultly"
#   4. writes a least-privilege policy and issues a periodic token for the app
#
# SECURITY: the unseal key and root token are stored in the private Docker volume
# mounted at /boot. In production use auto-unseal (cloud KMS / HSM) or split the
# Shamir shares between people, and never keep the root token around.
export BAO_ADDR="${BAO_ADDR:-http://openbao:8200}"
BOOT=/boot
TOKEN_DIR=/token
log() { echo "[bootstrap] $*"; }

while true; do
  status=$(bao status 2>/dev/null) || true
  initialized=$(echo "$status" | awk '/^Initialized/ {print $2}')
  sealed=$(echo "$status" | awk '/^Sealed/ {print $2}')

  if [ -z "$initialized" ]; then sleep 2; continue; fi

  if [ "$initialized" = "false" ]; then
    log "initialising OpenBao"
    out=$(bao operator init -key-shares=1 -key-threshold=1) || { sleep 3; continue; }
    echo "$out" | awk -F': ' '/^Unseal Key 1/ {print $2}' > "$BOOT/unseal-key"
    echo "$out" | awk -F': ' '/^Initial Root Token/ {print $2}' > "$BOOT/root-token"
    chmod 600 "$BOOT/unseal-key" "$BOOT/root-token"
    continue
  fi

  if [ "$sealed" = "true" ]; then
    bao operator unseal "$(cat "$BOOT/unseal-key")" >/dev/null && log "unsealed"
    sleep 1
    continue
  fi

  export BAO_TOKEN="$(cat "$BOOT/root-token")"
  if [ ! -f "$BOOT/configured" ]; then
    log "configuring Transit"
    bao secrets enable -path=transit transit >/dev/null 2>&1 || true
    bao write -f transit/keys/vaultly type=aes256-gcm96 >/dev/null
    bao policy write vaultly-app - >/dev/null <<'POLICY'
path "transit/datakey/plaintext/vaultly" { capabilities = ["update"] }
path "transit/encrypt/vaultly"           { capabilities = ["update"] }
path "transit/decrypt/vaultly"           { capabilities = ["update"] }
path "transit/rewrap/vaultly"            { capabilities = ["update"] }
POLICY
    touch "$BOOT/configured"
  fi

  if ! BAO_TOKEN="$(cat "$TOKEN_DIR/app-token" 2>/dev/null)" bao token lookup >/dev/null 2>&1; then
    log "issuing a new app token"
    tok=$(bao token create -policy=vaultly-app -period=768h -orphan -display-name=vaultly-app -format=json | sed -n 's/.*"client_token": *"\([^"]*\)".*/\1/p')
    if [ -n "$tok" ]; then
      echo "$tok" > "$TOKEN_DIR/app-token"
      chmod 644 "$TOKEN_DIR/app-token"
    fi
  fi
  sleep 10
done
