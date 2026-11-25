-- Keys owned by the application itself (PII encryption + blind index), wrapped by the KeyProvider.
CREATE TABLE IF NOT EXISTS app_keys (
  name text PRIMARY KEY,
  wrapped bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Users: email and name are stored encrypted; email_hash is a keyed blind index for lookups/uniqueness.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_enc text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_hash bytea;
ALTER TABLE users ADD COLUMN IF NOT EXISTS name_enc text;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ALTER COLUMN name DROP NOT NULL;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
CREATE UNIQUE INDEX IF NOT EXISTS users_email_hash_idx ON users(email_hash);

ALTER TABLE invites ADD COLUMN IF NOT EXISTS email_enc text;
ALTER TABLE invites ADD COLUMN IF NOT EXISTS email_hash bytea;
ALTER TABLE invites ALTER COLUMN email DROP NOT NULL;
CREATE INDEX IF NOT EXISTS invites_email_hash_idx ON invites(org_id, email_hash);
