-- Protected environments and change requests (four-eyes approval). Idempotent.
ALTER TABLE environments ADD COLUMN IF NOT EXISTS protected boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS change_requests (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id    uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  env_id        uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
  path          text NOT NULL,          -- sealed, like secrets.path
  key           text NOT NULL,          -- sealed, like secrets.key
  lookup_hash   bytea NOT NULL,         -- same keyed hash as secrets.lookup_hash
  op            text NOT NULL CHECK (op IN ('set','delete')),
  value_enc     bytea,                  -- proposed value, encrypted with the project data key
  comment       text NOT NULL DEFAULT '',  -- sealed
  base_version  int NOT NULL DEFAULT 0, -- version the author saw; 0 = secret did not exist
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','rejected','cancelled')),
  author_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  author_label  text NOT NULL,          -- sealed
  reviewer_id   uuid REFERENCES users(id) ON DELETE SET NULL,
  reviewer_label text,                  -- sealed
  note          text NOT NULL DEFAULT '',  -- sealed reviewer note
  created_at    timestamptz NOT NULL DEFAULT now(),
  decided_at    timestamptz
);
CREATE INDEX IF NOT EXISTS change_requests_project_idx ON change_requests(project_id, status, created_at DESC);
-- at most one open request per secret, so reviewers never race two proposals for the same key
CREATE UNIQUE INDEX IF NOT EXISTS change_requests_open_idx ON change_requests(env_id, lookup_hash) WHERE status = 'pending';
