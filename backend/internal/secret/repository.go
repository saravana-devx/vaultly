package secret

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

const cols = `s.id, s.key, s.path, s.version, s.comment, s.tags, s.expires_at, s.updated_at, s.updated_by`

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func scan(row pgx.Row, extra ...any) (Secret, error) {
	var s Secret
	dst := append([]any{&s.ID, &s.Key, &s.Path, &s.Version, &s.Comment, &s.Tags, &s.ExpiresAt, &s.UpdatedAt, &s.UpdatedBy}, extra...)
	err := row.Scan(dst...)
	return s, err
}

func (r *Repository) Get(ctx context.Context, projectID, id uuid.UUID) (*Stored, error) {
	var env string
	var ct []byte
	s, err := scan(r.db.QueryRow(ctx, `SELECT `+cols+`, s.encrypted_value, e.slug FROM secrets s JOIN environments e ON e.id=s.env_id
		WHERE s.id=$1 AND s.project_id=$2`, id, projectID), &ct, &env)
	if dberr.IsNoRows(err) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Env = env
	return &Stored{Secret: s, Ciphertext: ct}, nil
}

// List returns the secrets at path and below; the caller groups the deeper ones into folders.
func (r *Repository) List(ctx context.Context, envID uuid.UUID, path, tag string) ([]Secret, error) {
	args := []any{envID, path, strings.TrimRight(path, "/") + "/%"}
	q := `SELECT ` + cols + ` FROM secrets s WHERE s.env_id=$1 AND (s.path=$2 OR s.path LIKE $3)`
	if tag != "" {
		args = append(args, tag)
		q += ` AND $4 = ANY(s.tags)`
	}
	rows, err := r.db.Query(ctx, q+` ORDER BY s.key`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Secret
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Search(ctx context.Context, projectID uuid.UUID, envIDs []uuid.UUID, like string) ([]Secret, error) {
	rows, err := r.db.Query(ctx, `SELECT `+cols+`, e.slug FROM secrets s JOIN environments e ON e.id=s.env_id
		WHERE s.project_id=$1 AND s.env_id = ANY($2) AND (s.key ILIKE $3 OR s.comment ILIKE $3 OR s.path ILIKE $3 OR EXISTS (SELECT 1 FROM unnest(s.tags) t WHERE t ILIKE $3))
		ORDER BY s.key, e.position LIMIT 100`, projectID, envIDs, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Secret{}
	for rows.Next() {
		var env string
		s, err := scan(rows, &env)
		if err != nil {
			return nil, err
		}
		s.Env = env
		out = append(out, s)
	}
	return out, rows.Err()
}

// Upsert writes a new version inside one transaction. FOR UPDATE serialises concurrent writers
// of the same secret so version numbers never collide.
func (r *Repository) Upsert(ctx context.Context, w Write) (Secret, bool, error) {
	out := Secret{Key: w.Key, Path: w.Path}
	created := false
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var id uuid.UUID
		var version int
		var tags []string
		var expires *time.Time
		err := tx.QueryRow(ctx, `SELECT id, version, tags, expires_at FROM secrets WHERE env_id=$1 AND path=$2 AND key=$3 FOR UPDATE`,
			w.EnvID, w.Path, w.Key).Scan(&id, &version, &tags, &expires)
		exists := !errors.Is(err, pgx.ErrNoRows)
		if err != nil && exists {
			return err
		}
		if w.Tags != nil {
			tags = *w.Tags
		}
		if tags == nil {
			tags = []string{}
		}
		switch {
		case w.ClearExpiry:
			expires = nil
		case w.Expires != nil:
			expires = w.Expires
		}
		action := w.Action
		if exists {
			version++
			if action == "" {
				action = "update"
			}
			err = tx.QueryRow(ctx, `UPDATE secrets SET encrypted_value=$2, version=$3, comment=$4, tags=$5, expires_at=$6, expiry_notified=false,
				updated_by=$7, updated_at=now() WHERE id=$1 RETURNING updated_at`, id, w.Ciphertext, version, w.Comment, tags, expires, w.Actor).Scan(&out.UpdatedAt)
		} else {
			created, version = true, 1
			if action == "" {
				action = "create"
			}
			err = tx.QueryRow(ctx, `INSERT INTO secrets (project_id,env_id,path,key,encrypted_value,version,comment,tags,expires_at,updated_by)
				VALUES ($1,$2,$3,$4,$5,1,$6,$7,$8,$9) RETURNING id, updated_at`,
				w.ProjectID, w.EnvID, w.Path, w.Key, w.Ciphertext, w.Comment, tags, expires, w.Actor).Scan(&id, &out.UpdatedAt)
		}
		if err != nil {
			return err
		}
		out.ID, out.Version, out.Comment, out.Tags, out.ExpiresAt = id, version, w.Comment, tags, expires
		_, err = tx.Exec(ctx, `INSERT INTO secret_versions (secret_id,version,encrypted_value,action,actor) VALUES ($1,$2,$3,$4,$5)`,
			id, version, w.Ciphertext, action, w.Actor)
		return err
	})
	return out, created, err
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM secrets WHERE id=$1`, id)
	return err
}

func (r *Repository) Versions(ctx context.Context, id uuid.UUID) ([]VersionInfo, error) {
	rows, err := r.db.Query(ctx, `SELECT version,action,actor,created_at FROM secret_versions WHERE secret_id=$1 ORDER BY version DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VersionInfo{}
	for rows.Next() {
		var v VersionInfo
		if err := rows.Scan(&v.Version, &v.Action, &v.Actor, &v.At); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) VersionCiphertext(ctx context.Context, id uuid.UUID, version int) ([]byte, error) {
	var ct []byte
	err := r.db.QueryRow(ctx, `SELECT encrypted_value FROM secret_versions WHERE secret_id=$1 AND version=$2`, id, version).Scan(&ct)
	if dberr.IsNoRows(err) {
		return nil, ErrVersionNotFound
	}
	return ct, err
}

const live = ` AND (expires_at IS NULL OR expires_at > now())`

// Live returns non-expired secrets at path (and below if recursive); shallower paths come first
// so deeper ones win on key collisions.
func (r *Repository) Live(ctx context.Context, envID uuid.UUID, path string, recursive bool) ([]LiveRow, error) {
	q := `SELECT path, key, encrypted_value FROM secrets WHERE env_id=$1 AND path=$2` + live + ` ORDER BY key`
	args := []any{envID, path}
	if recursive {
		q = `SELECT path, key, encrypted_value FROM secrets WHERE env_id=$1 AND (path=$2 OR path LIKE $3)` + live + ` ORDER BY length(path), key`
		args = append(args, strings.TrimRight(path, "/")+"/%")
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LiveRow
	for rows.Next() {
		var lr LiveRow
		if err := rows.Scan(&lr.Path, &lr.Key, &lr.Ciphertext); err != nil {
			return nil, err
		}
		out = append(out, lr)
	}
	return out, rows.Err()
}

func (r *Repository) Lookup(ctx context.Context, envID uuid.UUID, path, key string) ([]byte, error) {
	var ct []byte
	err := r.db.QueryRow(ctx, `SELECT encrypted_value FROM secrets WHERE env_id=$1 AND path=$2 AND key=$3`+live, envID, path, key).Scan(&ct)
	return ct, err
}

// MarkExpired flags newly expired secrets as announced and returns them.
func (r *Repository) MarkExpired(ctx context.Context) ([]Expired, error) {
	rows, err := r.db.Query(ctx, `UPDATE secrets s SET expiry_notified = true FROM environments e, projects p
		WHERE e.id = s.env_id AND p.id = s.project_id AND s.expires_at < now() AND NOT s.expiry_notified
		RETURNING p.org_id, s.project_id, s.path, s.key, e.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Expired
	for rows.Next() {
		var e Expired
		if err := rows.Scan(&e.OrgID, &e.ProjectID, &e.Path, &e.Key, &e.Env); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
