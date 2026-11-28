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

// MFA returns the user's 2FA state as stored (secrets still sealed).
func (r *Repository) MFA(ctx context.Context, userID uuid.UUID) (*MFARecord, error) {
	m := &MFARecord{}
	var secret, pending *string
	err := r.db.QueryRow(ctx, `
		SELECT u.totp_secret_enc, u.totp_pending_enc, u.totp_last_step, u.totp_enabled_at,
		       (SELECT count(*) FROM mfa_recovery_codes c WHERE c.user_id=u.id AND c.used_at IS NULL)
		FROM users u WHERE u.id=$1`, userID).Scan(&secret, &pending, &m.LastStep, &m.EnabledAt, &m.RecoveryLeft)
	if dberr.IsNoRows(err) {
		return nil, ErrUserNotFound
	}
	if secret != nil {
		m.SecretEnc = *secret
	}
	if pending != nil {
		m.PendingEnc = *pending
	}
	return m, err
}

func (r *Repository) SetPendingTOTP(ctx context.Context, userID uuid.UUID, sealed string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET totp_pending_enc=$2 WHERE id=$1`, userID, sealed)
	return err
}

// EnableTOTP promotes the pending secret and replaces the recovery codes in one transaction.
func (r *Repository) EnableTOTP(ctx context.Context, userID uuid.UUID, step int64, codeHashes [][]byte) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET totp_secret_enc=totp_pending_enc, totp_pending_enc=NULL,
			totp_last_step=$2, totp_enabled_at=now() WHERE id=$1 AND totp_pending_enc IS NOT NULL`, userID, step)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrMFANoSetup
		}
		return replaceCodes(ctx, tx, userID, codeHashes)
	})
}

func (r *Repository) DisableTOTP(ctx context.Context, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET totp_secret_enc=NULL, totp_pending_enc=NULL, totp_last_step=0,
			totp_enabled_at=NULL WHERE id=$1`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, userID)
		return err
	})
}

func (r *Repository) ReplaceRecoveryCodes(ctx context.Context, userID uuid.UUID, codeHashes [][]byte) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error { return replaceCodes(ctx, tx, userID, codeHashes) })
}

func replaceCodes(ctx context.Context, tx pgx.Tx, userID uuid.UUID, hashes [][]byte) error {
	if _, err := tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := tx.Exec(ctx, `INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES ($1,$2)`, userID, h); err != nil {
			return err
		}
	}
	return nil
}

// AcceptTOTPStep records step as used. It is a compare-and-set, so two concurrent requests
// with the same code can't both succeed: only one sees a row updated.
func (r *Repository) AcceptTOTPStep(ctx context.Context, userID uuid.UUID, step int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE users SET totp_last_step=$2 WHERE id=$1 AND totp_last_step < $2`, userID, step)
	return err == nil && tag.RowsAffected() == 1, err
}

// UseRecoveryCode marks an unused code as used; false if it doesn't exist or was already used.
func (r *Repository) UseRecoveryCode(ctx context.Context, userID uuid.UUID, hash []byte) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE mfa_recovery_codes SET used_at=now() WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`, userID, hash)
	return err == nil && tag.RowsAffected() == 1, err
}
