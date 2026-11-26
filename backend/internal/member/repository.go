package member

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
	"vaultly/internal/pii"
)

// The environment name of a per-environment role is stored sealed; env_slug_hash is what lookups compare.
type Repository struct {
	db  *pgxpool.Pool
	box *pii.Box
}

func NewRepository(db *pgxpool.Pool, box *pii.Box) *Repository { return &Repository{db: db, box: box} }

func (r *Repository) List(ctx context.Context, org uuid.UUID) ([]Row, error) {
	rows, err := r.db.Query(ctx, `SELECT u.id,u.email_enc,u.name_enc,m.role,m.created_at FROM memberships m
		JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 ORDER BY m.created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var m Row
		if err := rows.Scan(&m.ID, &m.EmailEnc, &m.NameEnc, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) EnvOverrides(ctx context.Context, org uuid.UUID) (map[uuid.UUID]map[string]string, error) {
	rows, err := r.db.Query(ctx, `SELECT user_id, env_slug, role FROM member_env_roles WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]map[string]string{}
	for rows.Next() {
		var uid uuid.UUID
		var slug, role string
		if err := rows.Scan(&uid, &slug, &role); err != nil {
			return nil, err
		}
		slug = r.box.OpenName("member_env_roles.slug", slug)
		if out[uid] == nil {
			out[uid] = map[string]string{}
		}
		out[uid][slug] = role
	}
	return out, rows.Err()
}

func (r *Repository) Add(ctx context.Context, org, user uuid.UUID, role string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,$3)`, org, user, role)
	if dberr.IsUnique(err) {
		return ErrAlreadyMember
	}
	return err
}

func (r *Repository) SetRole(ctx context.Context, org, user uuid.UUID, role string) error {
	tag, err := r.db.Exec(ctx, `UPDATE memberships SET role=$3 WHERE org_id=$1 AND user_id=$2`, org, user, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *Repository) Remove(ctx context.Context, org, user uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, org, user)
	return err
}

// AdminInfo reports how many admins the workspace has and whether user is one of them.
func (r *Repository) AdminInfo(ctx context.Context, org, user uuid.UUID) (admins int, isAdmin bool, err error) {
	var any *bool
	err = r.db.QueryRow(ctx, `SELECT count(*), bool_or(user_id=$2) FROM memberships WHERE org_id=$1 AND role='admin'`, org, user).Scan(&admins, &any)
	return admins, any != nil && *any, err
}

func (r *Repository) EnvRoles(ctx context.Context, org, user uuid.UUID) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `SELECT env_slug, role FROM member_env_roles WHERE org_id=$1 AND user_id=$2`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slug, role string
		if err := rows.Scan(&slug, &role); err != nil {
			return nil, err
		}
		out[r.box.OpenName("member_env_roles.slug", slug)] = role
	}
	return out, rows.Err()
}

func (r *Repository) ReplaceEnvRoles(ctx context.Context, org, user uuid.UUID, roles map[string]string) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM member_env_roles WHERE org_id=$1 AND user_id=$2`, org, user); err != nil {
			return err
		}
		for slug, role := range roles {
			if _, err := tx.Exec(ctx, `INSERT INTO member_env_roles (org_id,user_id,env_slug,env_slug_hash,role) VALUES ($1,$2,$3,$5,$4)`, org, user, r.box.SealName("member_env_roles.slug", slug), role, r.box.Hash("environments.slug", slug)); err != nil {
				return err
			}
		}
		return nil
	})
	if dberr.IsForeignKey(err) {
		return ErrMemberNotFound
	}
	return err
}
