-- Field-level encryption of every human-readable column (see internal/pii).
-- Values are sealed by the application; the database only keeps ciphertext plus keyed hashes
-- for the few lookups it still has to do (equality and uniqueness). Idempotent.

-- environments: name and slug are sealed; slug_hash replaces the unique slug
ALTER TABLE environments ADD COLUMN IF NOT EXISTS slug_hash bytea;
ALTER TABLE environments DROP CONSTRAINT IF EXISTS environments_project_id_slug_key;
CREATE UNIQUE INDEX IF NOT EXISTS environments_slug_hash_idx ON environments(project_id, slug_hash);

-- secrets: path, key, comment and tags are sealed; hashes replace the unique (env, path, key)
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS path_hash bytea;
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS lookup_hash bytea;
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_env_id_path_key_key;
DROP INDEX IF EXISTS secrets_lookup_idx;
DROP INDEX IF EXISTS secrets_tags_idx;
CREATE UNIQUE INDEX IF NOT EXISTS secrets_lookup_hash_idx ON secrets(env_id, lookup_hash);
CREATE INDEX IF NOT EXISTS secrets_path_hash_idx ON secrets(env_id, path_hash);

-- columns that were arrays or JSON become text holding one sealed value
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT * FROM (VALUES
    ('secrets','tags','[]'), ('service_tokens','allowed_cidrs','[]'), ('webhooks','events','["*"]'),
    ('invites','env_roles','{}'), ('audit_logs','metadata','{}')
  ) AS t(tbl, col, dflt)
  LOOP
    IF (SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=r.tbl AND column_name=r.col) IN ('ARRAY','jsonb') THEN
      EXECUTE format('ALTER TABLE %I ALTER COLUMN %I DROP DEFAULT', r.tbl, r.col);
      EXECUTE format('ALTER TABLE %I ALTER COLUMN %I TYPE text USING coalesce(to_json(%I)::text, %L)', r.tbl, r.col, r.col, r.dflt);
      EXECUTE format('ALTER TABLE %I ALTER COLUMN %I SET DEFAULT %L', r.tbl, r.col, r.dflt);
    END IF;
  END LOOP;
END $$;
ALTER TABLE audit_logs ALTER COLUMN metadata DROP NOT NULL;

-- per-environment member roles: the slug is sealed, the hash is what we look up
ALTER TABLE member_env_roles ADD COLUMN IF NOT EXISTS env_slug_hash bytea;
CREATE INDEX IF NOT EXISTS member_env_roles_hash_idx ON member_env_roles(org_id, user_id, env_slug_hash);

-- dynamic secrets: the name is sealed, name_hash keeps it unique per environment
ALTER TABLE dynamic_configs ADD COLUMN IF NOT EXISTS name_hash bytea;
ALTER TABLE dynamic_configs DROP CONSTRAINT IF EXISTS dynamic_configs_project_id_env_id_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS dynamic_configs_name_hash_idx ON dynamic_configs(project_id, env_id, name_hash);
