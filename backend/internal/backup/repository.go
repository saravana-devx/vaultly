package backup

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) OrgName(ctx context.Context, org uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `SELECT name FROM orgs WHERE id=$1`, org).Scan(&name)
	if dberr.IsNoRows(err) {
		return "", ErrOrgNotFound
	}
	return name, err
}

// Export reads every project of the org with its environments, secrets and version history.
func (r *Repository) Export(ctx context.Context, org uuid.UUID) ([]exportProject, error) {
	prows, err := r.db.Query(ctx, `SELECT id,name,encrypted_dek,created_at FROM projects WHERE org_id=$1 ORDER BY name`, org)
	if err != nil {
		return nil, err
	}
	var projects []exportProject
	for prows.Next() {
		var p exportProject
		if err := prows.Scan(&p.ID, &p.Name, &p.EncryptedDEK, &p.CreatedAt); err != nil {
			prows.Close()
			return nil, err
		}
		projects = append(projects, p)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return nil, err
	}
	for i := range projects {
		if err := r.fill(ctx, &projects[i]); err != nil {
			return nil, err
		}
	}
	return projects, nil
}

func (r *Repository) fill(ctx context.Context, p *exportProject) error {
	erows, err := r.db.Query(ctx, `SELECT name,slug,position FROM environments WHERE project_id=$1 ORDER BY position`, p.ID)
	if err != nil {
		return err
	}
	for erows.Next() {
		var e exportEnv
		if err := erows.Scan(&e.Name, &e.Slug, &e.Position); err != nil {
			erows.Close()
			return err
		}
		p.Environments = append(p.Environments, e)
	}
	erows.Close()

	srows, err := r.db.Query(ctx, `SELECT s.id,e.slug,s.path,s.key,s.encrypted_value,s.version,s.comment,s.tags,s.expires_at,s.updated_by,s.updated_at
		FROM secrets s JOIN environments e ON e.id=s.env_id WHERE s.project_id=$1 ORDER BY e.position, s.path, s.key`, p.ID)
	if err != nil {
		return err
	}
	index := map[uuid.UUID]int{}
	for srows.Next() {
		var sec exportSecret
		if err := srows.Scan(&sec.id, &sec.Env, &sec.Path, &sec.Key, &sec.EncryptedValue, &sec.Version, &sec.Comment, &sec.Tags, &sec.ExpiresAt, &sec.UpdatedBy, &sec.UpdatedAt); err != nil {
			srows.Close()
			return err
		}
		index[sec.id] = len(p.Secrets)
		p.Secrets = append(p.Secrets, sec)
	}
	srows.Close()

	vrows, err := r.db.Query(ctx, `SELECT v.secret_id,v.version,v.encrypted_value,v.action,v.actor,v.created_at FROM secret_versions v
		JOIN secrets s ON s.id=v.secret_id WHERE s.project_id=$1 ORDER BY v.secret_id, v.version`, p.ID)
	if err != nil {
		return err
	}
	defer vrows.Close()
	for vrows.Next() {
		var sid uuid.UUID
		var v exportVersion
		if err := vrows.Scan(&sid, &v.Version, &v.EncryptedValue, &v.Action, &v.Actor, &v.CreatedAt); err != nil {
			return err
		}
		if idx, ok := index[sid]; ok {
			p.Secrets[idx].Versions = append(p.Secrets[idx].Versions, v)
		}
	}
	return vrows.Err()
}

func (r *Repository) ProjectExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE id=$1)`, id).Scan(&exists)
	return exists, err
}

// Restore inserts one project, its environments, secrets and versions in a single transaction
// and returns how many secrets it wrote.
func (r *Repository) Restore(ctx context.Context, org uuid.UUID, p exportProject) (int, error) {
	count := 0
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO projects (id,org_id,name,encrypted_dek,created_at) VALUES ($1,$2,$3,$4,$5)`, p.ID, org, p.Name, p.EncryptedDEK, p.CreatedAt); err != nil {
			return err
		}
		envIDs := map[string]uuid.UUID{}
		for _, e := range p.Environments {
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO environments (project_id,name,slug,position) VALUES ($1,$2,$3,$4) RETURNING id`, p.ID, e.Name, e.Slug, e.Position).Scan(&id); err != nil {
				return err
			}
			envIDs[e.Slug] = id
		}
		for _, sec := range p.Secrets {
			eid, ok := envIDs[sec.Env]
			if !ok {
				continue
			}
			if sec.Tags == nil {
				sec.Tags = []string{}
			}
			var sid uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO secrets (project_id,env_id,path,key,encrypted_value,version,comment,tags,expires_at,updated_by,updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
				p.ID, eid, sec.Path, sec.Key, sec.EncryptedValue, sec.Version, sec.Comment, sec.Tags, sec.ExpiresAt, sec.UpdatedBy, sec.UpdatedAt).Scan(&sid); err != nil {
				return err
			}
			for _, v := range sec.Versions {
				if _, err := tx.Exec(ctx, `INSERT INTO secret_versions (secret_id,version,encrypted_value,action,actor,created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
					sid, v.Version, v.EncryptedValue, v.Action, v.Actor, v.CreatedAt); err != nil {
					return err
				}
			}
			count++
		}
		return nil
	})
	return count, err
}
