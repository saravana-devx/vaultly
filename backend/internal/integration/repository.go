package integration

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, projectID uuid.UUID) ([]Row, error) {
	rows, err := r.db.Query(ctx, `SELECT i.id,i.name,i.provider,e.slug,i.path,i.auto_sync,i.config_enc,i.last_synced_at,i.last_status,i.last_error
		FROM integrations i JOIN environments e ON e.id=i.env_id WHERE i.project_id=$1 ORDER BY i.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var x Row
		if err := rows.Scan(&x.ID, &x.Name, &x.Provider, &x.Env, &x.Path, &x.AutoSync, &x.ConfigEnc, &x.LastSyncedAt, &x.LastStatus, &x.LastError); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, n NewIntegration) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO integrations (project_id,env_id,path,provider,name,config_enc,auto_sync) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		n.ProjectID, n.EnvID, n.Path, n.Provider, n.Name, n.ConfigEnc, n.AutoSync).Scan(&id)
	return id, err
}

func (r *Repository) SetAutoSync(ctx context.Context, id, projectID uuid.UUID, on bool) error {
	tag, err := r.db.Exec(ctx, `UPDATE integrations SET auto_sync=$3 WHERE id=$1 AND project_id=$2`, id, projectID, on)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id, projectID uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `DELETE FROM integrations WHERE id=$1 AND project_id=$2 RETURNING name`, id, projectID).Scan(&name)
	if dberr.IsNoRows(err) {
		return "", ErrNotFound
	}
	return name, err
}

func (r *Repository) SyncInfo(ctx context.Context, id uuid.UUID) (*SyncInfo, error) {
	s := &SyncInfo{ID: id}
	err := r.db.QueryRow(ctx, `SELECT i.project_id, p.org_id, i.env_id, i.name, i.provider, e.slug, i.path, i.config_enc
		FROM integrations i JOIN projects p ON p.id=i.project_id JOIN environments e ON e.id=i.env_id WHERE i.id=$1`, id).
		Scan(&s.ProjectID, &s.OrgID, &s.EnvID, &s.Name, &s.Provider, &s.EnvSlug, &s.Path, &s.ConfigEnc)
	if dberr.IsNoRows(err) {
		return nil, ErrNotFound
	}
	return s, err
}

func (r *Repository) MarkSynced(ctx context.Context, id uuid.UUID, status, msg string) {
	_, _ = r.db.Exec(ctx, `UPDATE integrations SET last_synced_at=now(), last_status=$2, last_error=$3 WHERE id=$1`, id, status, msg)
}

func (r *Repository) AutoSyncIDs(ctx context.Context, projectID uuid.UUID, env string) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `SELECT i.id FROM integrations i JOIN environments e ON e.id=i.env_id
		WHERE i.project_id=$1 AND i.auto_sync AND ($2 = '' OR e.slug=$2)`, projectID, env)
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
