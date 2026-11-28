package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
	"vaultly/internal/pii"
)

// Environment names and slugs are stored sealed; slug_hash keeps slugs unique within a project.
type Repository struct {
	db  *pgxpool.Pool
	box *pii.Box
}

func NewRepository(db *pgxpool.Pool, box *pii.Box) *Repository { return &Repository{db: db, box: box} }

func (r *Repository) slugHash(slug string) []byte { return r.box.Hash("environments.slug", slug) }
func (r *Repository) openEnv(e Env) Env {
	e.Name, e.Slug = r.box.OpenName("environments.name", e.Name), r.box.OpenName("environments.slug", e.Slug)
	return e
}

func (r *Repository) ListByOrg(ctx context.Context, org uuid.UUID) ([]Summary, error) {
	rows, err := r.db.Query(ctx, `SELECT p.id,p.name,p.created_at,
		(SELECT count(*) FROM secrets WHERE project_id=p.id),
		(SELECT max(created_at) FROM audit_logs WHERE project_id=p.id)
		FROM projects p WHERE p.org_id=$1 ORDER BY p.created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.Name, &s.CreatedAt, &s.SecretCount, &s.LastActivity); err != nil {
			return nil, err
		}
		s.Environments = []Env{}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) EnvsFor(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID][]Env, error) {
	rows, err := r.db.Query(ctx, `SELECT project_id,id,name,slug,protected FROM environments WHERE project_id = ANY($1) ORDER BY position, name`, projectIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]Env{}
	for rows.Next() {
		var pid uuid.UUID
		var e Env
		if err := rows.Scan(&pid, &e.ID, &e.Name, &e.Slug, &e.Protected); err != nil {
			return nil, err
		}
		out[pid] = append(out[pid], r.openEnv(e))
	}
	return out, rows.Err()
}

// Create stores the project with its wrapped data key and the default environments.
func (r *Repository) Create(ctx context.Context, org uuid.UUID, name string, wrappedKey []byte) (uuid.UUID, error) {
	var id uuid.UUID
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO projects (org_id,name,encrypted_dek) VALUES ($1,$2,$3) RETURNING id`, org, name, wrappedKey).Scan(&id); err != nil {
			return err
		}
		for i, e := range defaultEnvs {
			if _, err := tx.Exec(ctx, `INSERT INTO environments (project_id,name,slug,slug_hash,position) VALUES ($1,$2,$3,$5,$4)`, id, r.box.SealName("environments.name", e.Name), r.box.SealName("environments.slug", e.Slug), i, r.slugHash(e.Slug)); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM projects WHERE id=$1`, id)
	return err
}

func (r *Repository) CreateEnv(ctx context.Context, project uuid.UUID, name, slug string) (Env, error) {
	e := Env{}
	err := r.db.QueryRow(ctx, `INSERT INTO environments (project_id,name,slug,slug_hash,position)
		VALUES ($1,$2,$3,$4,(SELECT coalesce(max(position),0)+1 FROM environments WHERE project_id=$1)) RETURNING id`,
		project, r.box.SealName("environments.name", name), r.box.SealName("environments.slug", slug), r.slugHash(slug)).Scan(&e.ID)
	if dberr.IsUnique(err) {
		return Env{}, ErrEnvExists
	}
	e.Name, e.Slug = name, slug
	return e, err
}

func (r *Repository) DeleteEnv(ctx context.Context, project, env uuid.UUID) (string, error) {
	var slug string
	err := r.db.QueryRow(ctx, `DELETE FROM environments WHERE id=$1 AND project_id=$2 RETURNING slug`, env, project).Scan(&slug)
	if dberr.IsNoRows(err) {
		return "", ErrEnvNotFound
	}
	return r.box.OpenName("environments.slug", slug), err
}

func (r *Repository) SetEnvProtected(ctx context.Context, project, env uuid.UUID, on bool) (string, error) {
	var slug string
	err := r.db.QueryRow(ctx, `UPDATE environments SET protected=$3 WHERE id=$1 AND project_id=$2 RETURNING slug`, env, project, on).Scan(&slug)
	if dberr.IsNoRows(err) {
		return "", ErrEnvNotFound
	}
	return r.box.OpenName("environments.slug", slug), err
}
