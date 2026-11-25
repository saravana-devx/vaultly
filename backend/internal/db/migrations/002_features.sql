ALTER TABLE secrets ADD COLUMN IF NOT EXISTS tags text[] NOT NULL DEFAULT '{}';
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS expires_at timestamptz;
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS expiry_notified boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS secrets_expiry_idx ON secrets(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS secrets_tags_idx ON secrets USING gin(tags);

ALTER TABLE service_tokens ADD COLUMN IF NOT EXISTS allowed_cidrs text[] NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS member_env_roles (
  org_id uuid NOT NULL,
  user_id uuid NOT NULL,
  env_slug text NOT NULL,
  role text NOT NULL CHECK (role IN ('none','viewer','developer')),
  PRIMARY KEY (org_id, user_id, env_slug),
  FOREIGN KEY (org_id, user_id) REFERENCES memberships(org_id, user_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS invites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  email text NOT NULL,
  role text NOT NULL CHECK (role IN ('admin','developer','viewer')),
  env_roles jsonb NOT NULL DEFAULT '{}',
  token_hash bytea NOT NULL UNIQUE,
  invited_by text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS invites_org_idx ON invites(org_id);

CREATE TABLE IF NOT EXISTS personal_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  token_prefix text NOT NULL,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz,
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS webhooks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name text NOT NULL,
  url text NOT NULL,
  secret_enc bytea NOT NULL,
  events text[] NOT NULL DEFAULT '{*}',
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id bigserial PRIMARY KEY,
  webhook_id uuid NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
  event_id uuid NOT NULL,
  action text NOT NULL,
  status int NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  attempts int NOT NULL DEFAULT 1,
  duration_ms int NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS webhook_deliveries_idx ON webhook_deliveries(webhook_id, created_at DESC);

CREATE TABLE IF NOT EXISTS integrations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  env_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
  path text NOT NULL DEFAULT '/',
  provider text NOT NULL CHECK (provider IN ('github','vercel','railway')),
  name text NOT NULL,
  config_enc bytea NOT NULL,
  auto_sync boolean NOT NULL DEFAULT false,
  last_synced_at timestamptz,
  last_status text NOT NULL DEFAULT '',
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dynamic_configs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  env_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
  name text NOT NULL,
  provider text NOT NULL DEFAULT 'postgres',
  config_enc bytea NOT NULL,
  default_ttl_s int NOT NULL DEFAULT 3600,
  max_ttl_s int NOT NULL DEFAULT 86400,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, env_id, name)
);

CREATE TABLE IF NOT EXISTS dynamic_leases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  config_id uuid NOT NULL REFERENCES dynamic_configs(id) ON DELETE CASCADE,
  username text NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  revoke_error text NOT NULL DEFAULT '',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS dynamic_leases_active_idx ON dynamic_leases(expires_at) WHERE revoked_at IS NULL;
