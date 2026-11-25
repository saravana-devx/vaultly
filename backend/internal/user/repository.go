package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/dberr"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Create inserts the user together with their first workspace, where they are admin.
func (r *Repository) Create(ctx context.Context, u NewUser) (userID, orgID uuid.UUID, err error) {
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO users (email_enc,email_hash,name_enc,password_hash) VALUES ($1,$2,$3,$4) RETURNING id`,
			u.EmailEnc, u.EmailHash, u.NameEnc, u.PasswordHash).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO orgs (name) VALUES ($1) RETURNING id`, u.OrgName).Scan(&orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,'admin')`, orgID, userID)
		return err
	})
	if dberr.IsUnique(err) {
		return uuid.Nil, uuid.Nil, ErrEmailTaken
	}
	return userID, orgID, err
}

func (r *Repository) ByEmailHash(ctx context.Context, hash []byte) (*Record, error) {
	rec := &Record{}
	err := r.db.QueryRow(ctx, `SELECT id,email_enc,name_enc,password_hash FROM users WHERE email_hash=$1`, hash).
		Scan(&rec.ID, &rec.EmailEnc, &rec.NameEnc, &rec.PasswordHash)
	if dberr.IsNoRows(err) {
		return nil, ErrUserNotFound
	}
	return rec, err
}

func (r *Repository) ByID(ctx context.Context, id uuid.UUID) (*Record, error) {
	rec := &Record{ID: id}
	err := r.db.QueryRow(ctx, `SELECT email_enc,name_enc,password_hash FROM users WHERE id=$1`, id).
		Scan(&rec.EmailEnc, &rec.NameEnc, &rec.PasswordHash)
	if dberr.IsNoRows(err) {
		return nil, ErrUserNotFound
	}
	return rec, err
}

func (r *Repository) FirstOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, bool, error) {
	var org uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT org_id FROM memberships WHERE user_id=$1 ORDER BY created_at LIMIT 1`, userID).Scan(&org)
	if dberr.IsNoRows(err) {
		return uuid.Nil, false, nil
	}
	return org, err == nil, err
}
