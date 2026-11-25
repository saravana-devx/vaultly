package invite

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) IsMember(ctx context.Context, org uuid.UUID, emailHash []byte) (bool, error) {
	var found bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 AND u.email_hash=$2)`, org, emailHash).Scan(&found)
	return found, err
}

func (r *Repository) ReplacePending(ctx context.Context, n NewInvite) (uuid.UUID, error) {
	var id uuid.UUID
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM invites WHERE org_id=$1 AND email_hash=$2 AND accepted_at IS NULL`, n.OrgID, n.EmailHash); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO invites (org_id,email_enc,email_hash,role,env_roles,token_hash,invited_by,expires_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			n.OrgID, n.EmailEnc, n.EmailHash, n.Role, n.EnvRoles, n.TokenHash, n.InvitedBy, n.ExpiresAt).Scan(&id)
	})
	return id, err
}

func (r *Repository) OrgName(ctx context.Context, org uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `SELECT name FROM orgs WHERE id=$1`, org).Scan(&name)
	return name, err
}

func (r *Repository) ListPending(ctx context.Context, org uuid.UUID) ([]Invite, error) {
	rows, err := r.db.Query(ctx, `SELECT id,email_enc,role,env_roles,invited_by,expires_at,created_at FROM invites
		WHERE org_id=$1 AND accepted_at IS NULL ORDER BY created_at DESC`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		i := Invite{OrgID: org}
		if err := rows.Scan(&i.ID, &i.EmailEnc, &i.Role, &i.EnvRoles, &i.InvitedBy, &i.ExpiresAt, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Delete removes an invitation of the workspace and returns its sealed email for the audit trail.
func (r *Repository) Delete(ctx context.Context, id, org uuid.UUID) (string, error) {
	var emailEnc string
	err := r.db.QueryRow(ctx, `DELETE FROM invites WHERE id=$1 AND org_id=$2 RETURNING email_enc`, id, org).Scan(&emailEnc)
	if dberr.IsNoRows(err) {
		return "", ErrInviteNotFound
	}
	return emailEnc, err
}

func (r *Repository) ByTokenHash(ctx context.Context, hash []byte) (*Invite, error) {
	i := &Invite{}
	err := r.db.QueryRow(ctx, `SELECT i.id,i.org_id,o.name,i.email_enc,i.role,i.env_roles,i.invited_by,i.expires_at,i.accepted_at
		FROM invites i JOIN orgs o ON o.id=i.org_id WHERE i.token_hash=$1`, hash).
		Scan(&i.ID, &i.OrgID, &i.OrgName, &i.EmailEnc, &i.Role, &i.EnvRoles, &i.InvitedBy, &i.ExpiresAt, &i.Accepted)
	if dberr.IsNoRows(err) {
		return nil, ErrInviteNotFound
	}
	return i, err
}

// Accept creates the membership, applies the environment overrides and marks the invitation used.
func (r *Repository) Accept(ctx context.Context, i *Invite, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, i.OrgID, userID, i.Role); err != nil {
			return err
		}
		for slug, role := range i.EnvRoles {
			if _, err := tx.Exec(ctx, `INSERT INTO member_env_roles (org_id,user_id,env_slug,role) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, i.OrgID, userID, slug, role); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE invites SET accepted_at=now() WHERE id=$1`, i.ID)
		return err
	})
}
