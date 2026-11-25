CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  name text NOT NULL,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orgs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS memberships (
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('admin','developer','viewer')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, user_id)
);
CREATE INDEX IF NOT EXISTS memberships_user_idx ON memberships(user_id);

CREATE TABLE IF NOT EXISTS projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name text NOT NULL,
  encrypted_dek bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS projects_org_idx ON projects(org_id);

CREATE TABLE IF NOT EXISTS environments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name text NOT NULL,
  slug text NOT NULL,
  position int NOT NULL DEFAULT 0,
  UNIQUE (project_id, slug)
);

CREATE TABLE IF NOT EXISTS secrets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  env_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
  path text NOT NULL DEFAULT '/',
  key text NOT NULL,
  encrypted_value bytea NOT NULL,
  version int NOT NULL DEFAULT 1,
  comment text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text NOT NULL DEFAULT '',
  UNIQUE (env_id, path, key)
);
CREATE INDEX IF NOT EXISTS secrets_lookup_idx ON secrets(env_id, path);

CREATE TABLE IF NOT EXISTS secret_versions (
  id bigserial PRIMARY KEY,
  secret_id uuid NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
  version int NOT NULL,
  encrypted_value bytea NOT NULL,
  action text NOT NULL,
  actor text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (secret_id, version)
);

CREATE TABLE IF NOT EXISTS service_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  env_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
  name text NOT NULL,
  token_prefix text NOT NULL,
  token_hash bytea NOT NULL UNIQUE,
  path_prefix text NOT NULL DEFAULT '/',
  can_write boolean NOT NULL DEFAULT false,
  expires_at timestamptz,
  last_used_at timestamptz,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL,
  project_id uuid,
  actor_type text NOT NULL,
  actor_id text NOT NULL,
  actor_label text NOT NULL,
  action text NOT NULL,
  resource text NOT NULL DEFAULT '',
  metadata jsonb NOT NULL DEFAULT '{}',
  ip text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_org_time_idx ON audit_logs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_project_time_idx ON audit_logs(project_id, created_at DESC);
