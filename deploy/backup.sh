#!/bin/sh
# Back up everything needed to restore Vaultly: the database AND the key service.
# Run from the repo root on the server:  sh deploy/backup.sh
# Optional: BACKUP_PASSPHRASE="long passphrase" sh deploy/backup.sh   -> the archive is encrypted.
#
# !! The archive contains the OpenBao unseal key. Copy it OFF the server and keep it private.
set -eu
C="docker compose -f docker-compose.prod.yml"
STAMP=$(date +%Y%m%d-%H%M%S)
OUT="backups/$STAMP"
mkdir -p "$OUT"

echo "1/3 database"
$C exec -T postgres pg_dump -U vaultly vaultly | gzip > "$OUT/postgres.sql.gz"

echo "2/3 key service (raft snapshot + unseal material)"
$C exec -T openbao-bootstrap sh -c 'BAO_ADDR=http://openbao:8200 BAO_TOKEN=$(cat /boot/root-token) bao operator raft snapshot save /tmp/openbao.snap'
$C cp openbao-bootstrap:/tmp/openbao.snap "$OUT/openbao.snap"
$C cp openbao-bootstrap:/boot/unseal-key "$OUT/unseal-key"
$C cp openbao-bootstrap:/boot/root-token "$OUT/root-token"
chmod 600 "$OUT"/unseal-key "$OUT"/root-token

echo "3/3 archive"
tar czf "backups/vaultly-$STAMP.tgz" -C backups "$STAMP"
rm -rf "$OUT"
if [ -n "${BACKUP_PASSPHRASE:-}" ]; then
  openssl enc -aes-256-cbc -pbkdf2 -salt -pass env:BACKUP_PASSPHRASE -in "backups/vaultly-$STAMP.tgz" -out "backups/vaultly-$STAMP.tgz.enc"
  rm "backups/vaultly-$STAMP.tgz"
  echo "encrypted backup: backups/vaultly-$STAMP.tgz.enc"
else
  echo "backup: backups/vaultly-$STAMP.tgz   (UNENCRYPTED - set BACKUP_PASSPHRASE, and copy it off this server)"
fi
