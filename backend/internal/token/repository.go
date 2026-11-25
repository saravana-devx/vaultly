package token

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) ListService(ctx context.Context, projectID uuid.UUID) ([]ServiceView, error) {
	rows, err := r.db.Query(ctx, `SELECT t.id,t.name,t.token_prefix,e.slug,t.path_prefix,t.can_write,t.allowed_cidrs,t.expires_at,t.last_used_at,t.created_by,t.created_at
		FROM service_tokens t JOIN environments e ON e.id=t.env_id WHERE t.project_id=$1 ORDER BY t.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceView{}
	for rows.Next() {
		var v ServiceView
		if err := rows.Scan(&v.ID, &v.Name, &v.Prefix, &v.Env, &v.Path, &v.CanWrite, &v.AllowedCIDRs, &v.ExpiresAt, &v.LastUsedAt, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) CreateService(ctx context.Context, n NewServiceToken) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO service_tokens (project_id,env_id,name,token_prefix,token_hash,path_prefix,can_write,allowed_cidrs,expires_at,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		n.ProjectID, n.EnvID, n.Name, n.Prefix, n.Hash, n.Path, n.CanWrite, n.CIDRs, n.ExpiresAt, n.CreatedBy).Scan(&id)
	return id, err
}

func (r *Repository) DeleteService(ctx context.Context, id, projectID uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `DELETE FROM service_tokens WHERE id=$1 AND project_id=$2 RETURNING name`, id, projectID).Scan(&name)
	if dberr.IsNoRows(err) {
		return "", ErrTokenNotFound
	}
	return name, err
}

func (r *Repository) ServiceByHash(ctx context.Context, hash []byte) (*ServiceRecord, error) {
	t := &ServiceRecord{}
	err := r.db.QueryRow(ctx, `
		SELECT t.id, t.project_id, p.org_id, t.env_id, e.slug, t.name, t.path_prefix, t.can_write, t.allowed_cidrs, t.expires_at
		FROM service_tokens t JOIN projects p ON p.id = t.project_id JOIN environments e ON e.id = t.env_id
		WHERE t.token_hash = $1`, hash).
		Scan(&t.ID, &t.ProjectID, &t.OrgID, &t.EnvID, &t.EnvSlug, &t.Name, &t.PathPrefix, &t.CanWrite, &t.CIDRs, &t.ExpiresAt)
	if dberr.IsNoRows(err) {
		return nil, ErrInvalidToken
	}
	return t, err
}

func (r *Repository) TouchService(ctx context.Context, id uuid.UUID) {
	_, _ = r.db.Exec(ctx, `UPDATE service_tokens SET last_used_at = now() WHERE id = $1`, id)
}

func (r *Repository) ListPersonal(ctx context.Context, userID uuid.UUID) ([]PersonalView, error) {
	rows, err := r.db.Query(ctx, `SELECT id,name,token_prefix,expires_at,last_used_at,created_at FROM personal_tokens WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PersonalView{}
	for rows.Next() {
		var v PersonalView
		if err := rows.Scan(&v.ID, &v.Name, &v.Prefix, &v.ExpiresAt, &v.LastUsedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) CreatePersonal(ctx context.Context, n NewPersonalToken) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO personal_tokens (user_id,name,token_prefix,token_hash,expires_at) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		n.UserID, n.Name, n.Prefix, n.Hash, n.ExpiresAt).Scan(&id)
	return id, err
}

func (r *Repository) DeletePersonal(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM personal_tokens WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

func (r *Repository) PersonalByHash(ctx context.Context, hash []byte) (*PersonalRecord, error) {
	p := &PersonalRecord{}
	err := r.db.QueryRow(ctx, `SELECT t.id, u.id, u.email_enc, t.expires_at FROM personal_tokens t JOIN users u ON u.id=t.user_id WHERE t.token_hash=$1`, hash).
		Scan(&p.ID, &p.UserID, &p.EmailEnc, &p.ExpiresAt)
	if dberr.IsNoRows(err) {
		return nil, ErrInvalidToken
	}
	return p, err
}

func (r *Repository) TouchPersonal(ctx context.Context, id uuid.UUID) {
	_, _ = r.db.Exec(ctx, `UPDATE personal_tokens SET last_used_at = now() WHERE id = $1`, id)
}
