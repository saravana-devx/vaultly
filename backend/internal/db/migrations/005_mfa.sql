-- Two-factor sign-in (TOTP). The secret is sealed by the application (internal/pii);
-- recovery codes are stored as SHA-256 hashes. Idempotent.
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret_enc text;          -- confirmed secret, NULL = 2FA off
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_pending_enc text;         -- secret shown during setup, not yet confirmed
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_step bigint NOT NULL DEFAULT 0;  -- replay guard
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled_at timestamptz;

CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
  user_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash bytea NOT NULL,
  used_at   timestamptz,
  PRIMARY KEY (user_id, code_hash)
);
