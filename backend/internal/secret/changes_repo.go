package secret

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"vaultly/internal/dberr"
)

const (
	fChangeNote    = "change_requests.note"
	fChangeComment = "change_requests.comment"
)

type NewChange struct {
	ProjectID, EnvID, AuthorID uuid.UUID
	Path, Key, Op, Comment     string
	ValueEnc                   []byte
	BaseVersion                int
	AuthorLabel                string // already sealed
}

const changeCols = `c.id, c.env_id, e.slug, c.path, c.key, c.op, c.comment, c.base_version, c.status, c.author_id, c.author_label,
	c.reviewer_id, coalesce(c.reviewer_label,''), c.note, c.created_at, c.decided_at,
	coalesce((SELECT s.version FROM secrets s WHERE s.env_id=c.env_id AND s.lookup_hash=c.lookup_hash), 0)`

func (r *Repository) scanChange(row pgx.Row, extra ...any) (ChangeRequest, error) {
	var c ChangeRequest
	err := row.Scan(append([]any{&c.ID, &c.EnvID, &c.Env, &c.Path, &c.Key, &c.Op, &c.Comment, &c.BaseVersion, &c.Status, &c.AuthorID, &c.Author,
		&c.ReviewerID, &c.Reviewer, &c.Note, &c.CreatedAt, &c.DecidedAt, &c.CurrentVersion}, extra...)...)
	if err != nil {
		return c, err
	}
	c.Env, c.Path, c.Key = r.box.OpenName(fSlug, c.Env), r.box.OpenName(fPath, c.Path), r.box.OpenName(fKey, c.Key)
	c.Comment, c.Note = r.box.OpenName(fChangeComment, c.Comment), r.box.OpenName(fChangeNote, c.Note)
	c.Stale = c.Status == "pending" && c.CurrentVersion != c.BaseVersion
	return c, nil
}

func (r *Repository) CreateChange(ctx context.Context, n NewChange) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO change_requests (project_id,env_id,path,key,lookup_hash,op,value_enc,comment,base_version,author_id,author_label)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		n.ProjectID, n.EnvID, r.box.SealName(fPath, n.Path), r.box.SealName(fKey, n.Key), r.lookupHash(n.Path, n.Key), n.Op, n.ValueEnc,
		r.box.SealName(fChangeComment, n.Comment), n.BaseVersion, n.AuthorID, n.AuthorLabel).Scan(&id)
	if dberr.IsUnique(err) {
		return uuid.Nil, ErrChangeOpen
	}
	return id, err
}

// ListChanges returns a project's requests, newest first; status "" means all.
func (r *Repository) ListChanges(ctx context.Context, projectID uuid.UUID, status string) ([]ChangeRequest, error) {
	rows, err := r.db.Query(ctx, `SELECT `+changeCols+` FROM change_requests c JOIN environments e ON e.id=c.env_id
		WHERE c.project_id=$1 AND ($2='' OR c.status=$2) ORDER BY c.created_at DESC LIMIT 200`, projectID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChangeRequest{}
	for rows.Next() {
		c, err := r.scanChange(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) GetChange(ctx context.Context, projectID, id uuid.UUID) (*StoredChange, error) {
	sc := &StoredChange{}
	c, err := r.scanChange(r.db.QueryRow(ctx, `SELECT `+changeCols+`, c.value_enc FROM change_requests c JOIN environments e ON e.id=c.env_id
		WHERE c.project_id=$1 AND c.id=$2`, projectID, id), &sc.ValueEnc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrChangeNotFound
	}
	sc.ChangeRequest = c
	return sc, err
}

// DecideChange moves a request out of pending. It is a compare-and-set on status, so two
// reviewers clicking at once can't both apply it.
func (r *Repository) DecideChange(ctx context.Context, id uuid.UUID, status string, reviewer *uuid.UUID, reviewerLabel, note string) error {
	tag, err := r.db.Exec(ctx, `UPDATE change_requests SET status=$2, reviewer_id=$3, reviewer_label=$4, note=$5, decided_at=$6
		WHERE id=$1 AND status='pending'`, id, status, reviewer, reviewerLabel, r.box.SealName(fChangeNote, note), time.Now())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrChangeNotPending
	}
	return err
}

// ReopenChange puts a claimed request back to pending when applying it failed.
func (r *Repository) ReopenChange(ctx context.Context, id uuid.UUID) {
	_, _ = r.db.Exec(ctx, `UPDATE change_requests SET status='pending', reviewer_id=NULL, reviewer_label=NULL, note='', decided_at=NULL
		WHERE id=$1 AND status='applied'`, id)
}
