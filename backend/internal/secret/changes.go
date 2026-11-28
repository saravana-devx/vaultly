package secret

import (
	"context"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/pathutil"
	"vaultly/internal/principal"
)

// Change requests: on a protected environment, a non-admin's write or delete is stored as a
// proposal and only applied once another member with write access approves it.
//
//	pending ──approve──▶ applied
//	   ├────reject────▶ rejected
//	   └────cancel────▶ cancelled   (author only)

type ChangeRequest struct {
	ID             uuid.UUID  `json:"id"`
	EnvID          uuid.UUID  `json:"-"`
	Env            string     `json:"env"`
	Path           string     `json:"path"`
	Key            string     `json:"key"`
	Op             string     `json:"op"` // set | delete
	Comment        string     `json:"comment"`
	BaseVersion    int        `json:"base_version"`
	CurrentVersion int        `json:"current_version"`
	Stale          bool       `json:"stale"` // the secret changed after the request was opened
	Status         string     `json:"status"`
	AuthorID       uuid.UUID  `json:"author_id"`
	Author         string     `json:"author"`
	ReviewerID     *uuid.UUID `json:"reviewer_id"`
	Reviewer       string     `json:"reviewer"`
	Note           string     `json:"note"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at"`
}

type StoredChange struct {
	ChangeRequest
	ValueEnc []byte
}

// ChangeDetail adds the values a reviewer needs to judge the change. Opening one is audited.
type ChangeDetail struct {
	ChangeRequest
	Proposed *string `json:"proposed"`
	Current  *string `json:"current"`
}

type ChangeRepository interface {
	CreateChange(ctx context.Context, n NewChange) (uuid.UUID, error)
	ListChanges(ctx context.Context, projectID uuid.UUID, status string) ([]ChangeRequest, error)
	GetChange(ctx context.Context, projectID, id uuid.UUID) (*StoredChange, error)
	DecideChange(ctx context.Context, id uuid.UUID, status string, reviewer *uuid.UUID, reviewerLabel, note string) error
	ReopenChange(ctx context.Context, id uuid.UUID)
	VersionAt(ctx context.Context, envID uuid.UUID, path, key string) (int, error)
	DeleteAt(ctx context.Context, envID uuid.UUID, path, key string, version int) error
}

// gated reports whether this caller's change to envID must become a change request.
func (s *Service) gated(ctx context.Context, sc *scope, envID uuid.UUID) (bool, error) {
	return s.access.NeedsApproval(ctx, sc.p, envID)
}

func (s *Service) propose(ctx context.Context, sc *scope, env string, envID uuid.UUID, path, key, op, value, comment string) (*ChangeRequest, error) {
	base, err := s.repo.VersionAt(ctx, envID, path, key)
	if err != nil {
		return nil, err
	}
	n := NewChange{ProjectID: sc.p.ID, EnvID: envID, AuthorID: sc.actor.UserID, Path: path, Key: key, Op: op, Comment: comment,
		BaseVersion: base, AuthorLabel: s.pii.Label(sc.actor.Label())}
	if op == "set" {
		if n.ValueEnc, err = s.encrypt(ctx, sc.p.ID, value); err != nil {
			return nil, err
		}
	}
	id, err := s.repo.CreateChange(ctx, n)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, sc.actor, sc.p.OrgID, &sc.p.ID, "change.request", pathutil.Join(path, key), map[string]any{"env": env, "op": op, "change_request": id.String()})
	st, err := s.repo.GetChange(ctx, sc.p.ID, id)
	if err != nil {
		return nil, err
	}
	s.tidyChange(&st.ChangeRequest)
	return &st.ChangeRequest, nil
}

func (s *Service) tidyChange(c *ChangeRequest) {
	c.Author, c.Reviewer = s.pii.OpenLabel(c.Author), s.pii.OpenLabel(c.Reviewer)
}

// Changes lists requests in environments the caller can read.
func (s *Service) Changes(ctx context.Context, a principal.Actor, projectID uuid.UUID, status string) ([]ChangeRequest, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	switch status {
	case "", "pending", "applied", "rejected", "cancelled":
	default:
		return nil, ErrInvalidChangeStatus
	}
	readable, err := s.access.ReadableEnvs(ctx, sc.p, a.UserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListChanges(ctx, projectID, status)
	if err != nil {
		return nil, err
	}
	out := make([]ChangeRequest, 0, len(rows))
	for _, c := range rows {
		if _, ok := readable[c.Env]; ok {
			s.tidyChange(&c)
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Service) loadChange(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, need string) (*scope, *StoredChange, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, nil, err
	}
	st, err := s.repo.GetChange(ctx, projectID, id)
	if err != nil {
		return nil, nil, err
	}
	if err := s.need(ctx, sc, st.Env, need); err != nil {
		return nil, nil, err
	}
	s.tidyChange(&st.ChangeRequest)
	return sc, st, nil
}

// Change returns one request with the proposed and current values, so a reviewer can compare them.
func (s *Service) Change(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) (*ChangeDetail, error) {
	sc, st, err := s.loadChange(ctx, a, projectID, id, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	out := &ChangeDetail{ChangeRequest: st.ChangeRequest}
	if st.ValueEnc != nil {
		v, err := s.decrypt(ctx, projectID, st.ValueEnc)
		if err != nil {
			return nil, err
		}
		out.Proposed = &v
	}
	if ct, err := s.repo.Lookup(ctx, st.EnvID, st.Path, st.Key); err == nil {
		v, err := s.decrypt(ctx, projectID, ct)
		if err != nil {
			return nil, err
		}
		out.Current = &v
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "change.view", pathutil.Join(st.Path, st.Key), map[string]any{"env": st.Env, "change_request": id.String()})
	return out, nil
}

// Approve applies a pending request. The reviewer must be someone else with write access to that
// environment. The write is conditional on the version the author saw, so a request that went
// stale fails with a conflict instead of silently overwriting a newer value.
func (s *Service) Approve(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, note string) (*ChangeRequest, error) {
	sc, st, err := s.loadChange(ctx, a, projectID, id, access.RoleDeveloper)
	if err != nil {
		return nil, err
	}
	if st.AuthorID == a.UserID {
		return nil, ErrSelfApproval
	}
	if st.Status != "pending" {
		return nil, ErrChangeNotPending
	}
	if st.Stale {
		return nil, ErrStaleChange
	}
	reviewer := a.UserID
	if err := s.repo.DecideChange(ctx, id, "applied", &reviewer, s.pii.Label(a.Label()), note); err != nil {
		return nil, err // someone else decided first
	}
	version, err := s.applyChange(ctx, sc, st)
	if err != nil {
		s.repo.ReopenChange(ctx, id)
		return nil, err
	}
	res := pathutil.Join(st.Path, st.Key)
	md := map[string]any{"env": st.Env, "change_request": id.String(), "author": st.Author}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "change.approve", res, md)
	// The normal secret event too, so webhooks, integrations and the audit trail see an ordinary change.
	action := map[bool]string{true: "secret.create", false: "secret.update"}[st.BaseVersion == 0]
	if st.Op == "delete" {
		action = "secret.delete"
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, action, res, map[string]any{"env": st.Env, "version": version, "change_request": id.String(), "approved_by": a.Label()})
	return s.reload(ctx, projectID, id)
}

func (s *Service) applyChange(ctx context.Context, sc *scope, st *StoredChange) (int, error) {
	if st.Op == "delete" {
		return 0, s.repo.DeleteAt(ctx, st.EnvID, st.Path, st.Key, st.BaseVersion)
	}
	value, err := s.decrypt(ctx, sc.p.ID, st.ValueEnc)
	if err != nil {
		return 0, err
	}
	// Recorded under the author's name in the version history: it is their change.
	sec, _, err := s.write(ctx, Write{ProjectID: sc.p.ID, EnvID: st.EnvID, Path: st.Path, Key: st.Key, Comment: st.Comment,
		KeepComment: st.Comment == "", Actor: s.pii.Label(st.Author), CheckVersion: true, ExpectVersion: st.BaseVersion}, value)
	return sec.Version, err
}

func (s *Service) Reject(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, note string) (*ChangeRequest, error) {
	sc, st, err := s.loadChange(ctx, a, projectID, id, access.RoleDeveloper)
	if err != nil {
		return nil, err
	}
	if st.AuthorID == a.UserID {
		return nil, ErrSelfReview
	}
	reviewer := a.UserID
	if err := s.repo.DecideChange(ctx, id, "rejected", &reviewer, s.pii.Label(a.Label()), note); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "change.reject", pathutil.Join(st.Path, st.Key), map[string]any{"env": st.Env, "change_request": id.String(), "author": st.Author})
	return s.reload(ctx, projectID, id)
}

func (s *Service) CancelChange(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) (*ChangeRequest, error) {
	sc, st, err := s.loadChange(ctx, a, projectID, id, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	if st.AuthorID != a.UserID {
		return nil, ErrNotAuthor
	}
	if err := s.repo.DecideChange(ctx, id, "cancelled", nil, "", ""); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "change.cancel", pathutil.Join(st.Path, st.Key), map[string]any{"env": st.Env, "change_request": id.String()})
	return s.reload(ctx, projectID, id)
}

func (s *Service) reload(ctx context.Context, projectID, id uuid.UUID) (*ChangeRequest, error) {
	st, err := s.repo.GetChange(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	s.tidyChange(&st.ChangeRequest)
	return &st.ChangeRequest, nil
}
