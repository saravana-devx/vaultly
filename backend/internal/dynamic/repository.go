package dynamic

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, projectID uuid.UUID) ([]ConfigRow, error) {
	rows, err := r.db.Query(ctx, `SELECT d.id,d.name,d.provider,e.slug,d.config_enc,d.default_ttl_s,d.max_ttl_s,
		(SELECT count(*) FROM dynamic_leases l WHERE l.config_id=d.id AND l.revoked_at IS NULL AND l.expires_at > now())
		FROM dynamic_configs d JOIN environments e ON e.id=d.env_id WHERE d.project_id=$1 ORDER BY d.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfigRow{}
	for rows.Next() {
		c := ConfigRow{ProjectID: projectID}
		if err := rows.Scan(&c.ID, &c.Name, &c.Provider, &c.Env, &c.ConfigEnc, &c.DefaultTTL, &c.MaxTTL, &c.Active); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, n NewConfig) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO dynamic_configs (project_id,env_id,name,config_enc,default_ttl_s,max_ttl_s) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		n.ProjectID, n.EnvID, n.Name, n.ConfigEnc, n.DefaultTTL, n.MaxTTL).Scan(&id)
	if dberr.IsUnique(err) {
		return uuid.Nil, ErrNameTaken
	}
	return id, err
}

func (r *Repository) Name(ctx context.Context, id, projectID uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `SELECT name FROM dynamic_configs WHERE id=$1 AND project_id=$2`, id, projectID).Scan(&name)
	if dberr.IsNoRows(err) {
		return "", ErrNotFound
	}
	return name, err
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM dynamic_configs WHERE id=$1`, id)
	return err
}

// ConfigForLease returns a definition with its environment slug, scoped to the project.
func (r *Repository) ConfigForLease(ctx context.Context, id, projectID uuid.UUID) (*ConfigRow, error) {
	c := &ConfigRow{ID: id, ProjectID: projectID}
	err := r.db.QueryRow(ctx, `SELECT d.name, e.slug, d.config_enc, d.default_ttl_s, d.max_ttl_s FROM dynamic_configs d JOIN environments e ON e.id=d.env_id
		WHERE d.id=$1 AND d.project_id=$2`, id, projectID).Scan(&c.Name, &c.Env, &c.ConfigEnc, &c.DefaultTTL, &c.MaxTTL)
	if dberr.IsNoRows(err) {
		return nil, ErrNotFound
	}
	return c, err
}

func (r *Repository) IDByName(ctx context.Context, projectID, envID uuid.UUID, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT id FROM dynamic_configs WHERE project_id=$1 AND env_id=$2 AND name=$3`, projectID, envID, name).Scan(&id)
	if dberr.IsNoRows(err) {
		return uuid.Nil, ErrTokenNotFound
	}
	return id, err
}

func (r *Repository) InsertLease(ctx context.Context, configID uuid.UUID, username string, exp time.Time, createdBy string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO dynamic_leases (config_id,username,expires_at,created_by) VALUES ($1,$2,$3,$4) RETURNING id`,
		configID, username, exp, createdBy).Scan(&id)
	return id, err
}

func (r *Repository) Leases(ctx context.Context, configID, projectID uuid.UUID) ([]LeaseView, error) {
	rows, err := r.db.Query(ctx, `SELECT l.id,l.username,l.expires_at,l.revoked_at,l.created_by,l.created_at,l.revoke_error FROM dynamic_leases l
		JOIN dynamic_configs d ON d.id=l.config_id WHERE d.id=$1 AND d.project_id=$2 ORDER BY l.created_at DESC LIMIT 30`, configID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaseView{}
	for rows.Next() {
		var l LeaseView
		if err := rows.Scan(&l.ID, &l.Username, &l.ExpiresAt, &l.RevokedAt, &l.CreatedBy, &l.CreatedAt, &l.RevokeError); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ActiveLease returns the revoke target of a lease that is still live, or nil.
func (r *Repository) ActiveLease(ctx context.Context, leaseID uuid.UUID) (*LeaseTarget, error) {
	t := &LeaseTarget{}
	err := r.db.QueryRow(ctx, `SELECT l.username, d.config_enc, d.project_id FROM dynamic_leases l JOIN dynamic_configs d ON d.id=l.config_id
		WHERE l.id=$1 AND l.revoked_at IS NULL`, leaseID).Scan(&t.Username, &t.ConfigEnc, &t.ProjectID)
	if dberr.IsNoRows(err) {
		return nil, nil
	}
	return t, err
}

func (r *Repository) MarkRevoked(ctx context.Context, leaseID uuid.UUID) {
	_, _ = r.db.Exec(ctx, `UPDATE dynamic_leases SET revoked_at=now(), revoke_error='' WHERE id=$1`, leaseID)
}

func (r *Repository) MarkRevokeFailed(ctx context.Context, leaseID uuid.UUID, msg string) {
	_, _ = r.db.Exec(ctx, `UPDATE dynamic_leases SET revoke_error=$2 WHERE id=$1`, leaseID, msg)
}

func (r *Repository) leaseIDs(ctx context.Context, where string, arg uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `SELECT l.id FROM dynamic_leases l JOIN dynamic_configs d ON d.id=l.config_id WHERE l.revoked_at IS NULL AND `+where, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) LeaseIDsForConfig(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	return r.leaseIDs(ctx, `d.id=$1`, id)
}

func (r *Repository) LeaseIDsForProject(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	return r.leaseIDs(ctx, `d.project_id=$1`, id)
}

func (r *Repository) LeaseIDsForEnv(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	return r.leaseIDs(ctx, `d.env_id=$1`, id)
}

func (r *Repository) ExpiredLeaseIDs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM dynamic_leases WHERE revoked_at IS NULL AND expires_at < now() ORDER BY expires_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
