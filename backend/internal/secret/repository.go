package secret

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
	"vaultly/internal/pii"
)

// A secret's key, path, comment and tags are stored sealed. The database can only compare two
// keyed hashes (path_hash, lookup_hash), so folder listing and search filter decrypted rows here.
const (
	fPath    = "secrets.path"
	fKey     = "secrets.key"
	fComment = "secrets.comment"
	fTags    = "secrets.tags"
	fLookup  = "secrets.lookup"
	fSlug    = "environments.slug"
)

const cols = `s.id, s.key, s.path, s.version, s.comment, s.tags, s.expires_at, s.updated_at, s.updated_by`

type Repository struct {
	db  *pgxpool.Pool
	box *pii.Box
}

func NewRepository(db *pgxpool.Pool, box *pii.Box) *Repository { return &Repository{db: db, box: box} }

func (r *Repository) pathHash(path string) []byte { return r.box.Hash(fPath, path) }
func (r *Repository) lookupHash(path, key string) []byte {
	return r.box.Hash(fLookup, path+"\x00"+key)
}

func (r *Repository) scan(row pgx.Row, extra ...any) (Secret, error) {
	var s Secret
	var key, path, comment, tags string
	dst := append([]any{&s.ID, &key, &path, &s.Version, &comment, &tags, &s.ExpiresAt, &s.UpdatedAt, &s.UpdatedBy}, extra...)
	if err := row.Scan(dst...); err != nil {
		return s, err
	}
	s.Key, s.Path, s.Comment = r.box.OpenName(fKey, key), r.box.OpenName(fPath, path), r.box.OpenName(fComment, comment)
	s.Tags = r.openTags(tags)
	return s, nil
}

func (r *Repository) openTags(raw string) []string {
	tags := []string{}
	if err := r.box.OpenJSON(fTags, raw, &tags); err != nil || tags == nil {
		return []string{}
	}
	return tags
}

func (r *Repository) Get(ctx context.Context, projectID, id uuid.UUID) (*Stored, error) {
	var env string
	var ct []byte
	s, err := r.scan(r.db.QueryRow(ctx, `SELECT `+cols+`, s.encrypted_value, e.slug FROM secrets s JOIN environments e ON e.id=s.env_id
		WHERE s.id=$1 AND s.project_id=$2`, id, projectID), &ct, &env)
	if dberr.IsNoRows(err) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Env = r.box.OpenName(fSlug, env)
	return &Stored{Secret: s, Ciphertext: ct}, nil
}

func (r *Repository) envRows(ctx context.Context, envID uuid.UUID) ([]Secret, error) {
	rows, err := r.db.Query(ctx, `SELECT `+cols+` FROM secrets s WHERE s.env_id=$1`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Secret
	for rows.Next() {
		s, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// List returns the secrets at path and below; the caller groups the deeper ones into folders.
func (r *Repository) List(ctx context.Context, envID uuid.UUID, path, tag string) ([]Secret, error) {
	all, err := r.envRows(ctx, envID)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimRight(path, "/") + "/"
	var out []Secret
	for _, s := range all {
		if s.Path != path && !strings.HasPrefix(s.Path, prefix) {
			continue
		}
		if tag != "" && !slices.Contains(s.Tags, tag) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Key < out[b].Key })
	return out, nil
}

// Search matches the lower-cased needle against key, comment, path and tags of the given environments.
func (r *Repository) Search(ctx context.Context, projectID uuid.UUID, envIDs []uuid.UUID, needle string) ([]Secret, error) {
	rows, err := r.db.Query(ctx, `SELECT `+cols+`, e.slug, e.position FROM secrets s JOIN environments e ON e.id=s.env_id
		WHERE s.project_id=$1 AND s.env_id = ANY($2)`, projectID, envIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type hit struct {
		s   Secret
		pos int
	}
	var hits []hit
	for rows.Next() {
		var env string
		var pos int
		s, err := r.scan(rows, &env, &pos)
		if err != nil {
			return nil, err
		}
		s.Env = r.box.OpenName(fSlug, env)
		if matches(s, needle) {
			hits = append(hits, hit{s, pos})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(a, b int) bool {
		if hits[a].s.Key != hits[b].s.Key {
			return hits[a].s.Key < hits[b].s.Key
		}
		return hits[a].pos < hits[b].pos
	})
	out := []Secret{}
	for i := 0; i < len(hits) && i < 100; i++ {
		out = append(out, hits[i].s)
	}
	return out, nil
}

func matches(s Secret, needle string) bool {
	has := func(v string) bool { return strings.Contains(strings.ToLower(v), needle) }
	if has(s.Key) || has(s.Comment) || has(s.Path) {
		return true
	}
	return slices.ContainsFunc(s.Tags, has)
}

// Upsert writes a new version inside one transaction. FOR UPDATE serialises concurrent writers
// of the same secret so version numbers never collide.
func (r *Repository) Upsert(ctx context.Context, w Write) (Secret, bool, error) {
	out := Secret{Key: w.Key, Path: w.Path}
	created := false
	lookup := r.lookupHash(w.Path, w.Key)
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var id uuid.UUID
		var version int
		var tagsRaw string
		var expires *time.Time
		err := tx.QueryRow(ctx, `SELECT id, version, tags, expires_at FROM secrets WHERE env_id=$1 AND lookup_hash=$2 FOR UPDATE`,
			w.EnvID, lookup).Scan(&id, &version, &tagsRaw, &expires)
		exists := !errors.Is(err, pgx.ErrNoRows)
		if err != nil && exists {
			return err
		}
		tags := r.openTags(tagsRaw)
		if w.Tags != nil {
			tags = *w.Tags
		}
		switch {
		case w.ClearExpiry:
			expires = nil
		case w.Expires != nil:
			expires = w.Expires
		}
		action := w.Action
		sealedComment, sealedTags := r.box.SealName(fComment, w.Comment), r.box.SealJSON(fTags, tags)
		if exists {
			version++
			if action == "" {
				action = "update"
			}
			err = tx.QueryRow(ctx, `UPDATE secrets SET encrypted_value=$2, version=$3, comment=$4, tags=$5, expires_at=$6, expiry_notified=false,
				updated_by=$7, updated_at=now() WHERE id=$1 RETURNING updated_at`, id, w.Ciphertext, version, sealedComment, sealedTags, expires, w.Actor).Scan(&out.UpdatedAt)
		} else {
			created, version = true, 1
			if action == "" {
				action = "create"
			}
			err = tx.QueryRow(ctx, `INSERT INTO secrets (project_id,env_id,path,key,path_hash,lookup_hash,encrypted_value,version,comment,tags,expires_at,updated_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,1,$8,$9,$10,$11) RETURNING id, updated_at`,
				w.ProjectID, w.EnvID, r.box.SealName(fPath, w.Path), r.box.SealName(fKey, w.Key), r.pathHash(w.Path), lookup,
				w.Ciphertext, sealedComment, sealedTags, expires, w.Actor).Scan(&id, &out.UpdatedAt)
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
	q := `SELECT path, key, encrypted_value FROM secrets WHERE env_id=$1` + live
	args := []any{envID}
	if !recursive {
		q += ` AND path_hash=$2`
		args = append(args, r.pathHash(path))
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prefix := strings.TrimRight(path, "/") + "/"
	var out []LiveRow
	for rows.Next() {
		var p, k string
		var ct []byte
		if err := rows.Scan(&p, &k, &ct); err != nil {
			return nil, err
		}
		lr := LiveRow{Path: r.box.OpenName(fPath, p), Key: r.box.OpenName(fKey, k), Ciphertext: ct}
		if recursive && lr.Path != path && !strings.HasPrefix(lr.Path, prefix) {
			continue
		}
		out = append(out, lr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(a, b int) bool {
		if len(out[a].Path) != len(out[b].Path) {
			return len(out[a].Path) < len(out[b].Path)
		}
		return out[a].Key < out[b].Key
	})
	return out, nil
}

func (r *Repository) Lookup(ctx context.Context, envID uuid.UUID, path, key string) ([]byte, error) {
	var ct []byte
	err := r.db.QueryRow(ctx, `SELECT encrypted_value FROM secrets WHERE env_id=$1 AND lookup_hash=$2`+live, envID, r.lookupHash(path, key)).Scan(&ct)
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
		e.Path, e.Key, e.Env = r.box.OpenName(fPath, e.Path), r.box.OpenName(fKey, e.Key), r.box.OpenName(fSlug, e.Env)
		out = append(out, e)
	}
	return out, rows.Err()
}
