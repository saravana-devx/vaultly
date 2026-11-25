package token

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/pathutil"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
	"vaultly/internal/tokenutil"
)

type TokenRepository interface {
	ListService(ctx context.Context, projectID uuid.UUID) ([]ServiceView, error)
	CreateService(ctx context.Context, n NewServiceToken) (uuid.UUID, error)
	DeleteService(ctx context.Context, id, projectID uuid.UUID) (string, error)
	ServiceByHash(ctx context.Context, hash []byte) (*ServiceRecord, error)
	TouchService(ctx context.Context, id uuid.UUID)
	ListPersonal(ctx context.Context, userID uuid.UUID) ([]PersonalView, error)
	CreatePersonal(ctx context.Context, n NewPersonalToken) (uuid.UUID, error)
	DeletePersonal(ctx context.Context, id, userID uuid.UUID) error
	PersonalByHash(ctx context.Context, hash []byte) (*PersonalRecord, error)
	TouchPersonal(ctx context.Context, id uuid.UUID)
}

type Service struct {
	repo   TokenRepository
	access *access.Service
	pii    *pii.Box
	audit  *audit.Service
}

func NewService(repo TokenRepository, access *access.Service, box *pii.Box, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, pii: box, audit: audit}
}

func expiry(days int) *time.Time {
	if days <= 0 {
		return nil
	}
	t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	return &t
}

func (s *Service) ListService(ctx context.Context, a principal.Actor, projectID uuid.UUID) ([]ServiceView, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	out, err := s.repo.ListService(ctx, projectID)
	for i := range out {
		out[i].CreatedBy = s.pii.OpenLabel(out[i].CreatedBy)
	}
	return out, err
}

func (s *Service) CreateService(ctx context.Context, a principal.Actor, projectID uuid.UUID, req *CreateServiceRequest) (*Issued, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	path, ok := pathutil.Norm(req.Path)
	envID, envErr := s.access.EnvID(ctx, projectID, req.Env)
	if !ok || envErr != nil {
		return nil, ErrInvalidEnvPath
	}
	cidrs, ok := normCIDRs(trimAll(req.AllowedCIDRs))
	if !ok {
		return nil, ErrInvalidCIDRs
	}
	raw, hash := tokenutil.New(tokenutil.PrefixService)
	id, err := s.repo.CreateService(ctx, NewServiceToken{ProjectID: projectID, EnvID: envID, Name: name, Prefix: raw[:10], Hash: hash, Path: path,
		CanWrite: req.CanWrite, CIDRs: cidrs, ExpiresAt: expiry(req.ExpireDays), CreatedBy: s.pii.Label(a.Label())})
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "token.create", name,
		map[string]any{"env": req.Env, "path": path, "can_write": req.CanWrite, "ip_restricted": len(cidrs) > 0})
	return &Issued{ID: id, Token: raw, Name: name}, nil
}

func (s *Service) RevokeService(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return err
	}
	name, err := s.repo.DeleteService(ctx, id, projectID)
	if err != nil {
		return err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "token.revoke", name, nil)
	return nil
}

func (s *Service) ListPersonal(ctx context.Context, a principal.Actor) ([]PersonalView, error) {
	return s.repo.ListPersonal(ctx, a.UserID)
}

func (s *Service) CreatePersonal(ctx context.Context, a principal.Actor, req *CreatePersonalRequest) (*Issued, error) {
	if a.ViaPAT {
		return nil, ErrNeedPassword
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	raw, hash := tokenutil.New(tokenutil.PrefixPersonal)
	id, err := s.repo.CreatePersonal(ctx, NewPersonalToken{UserID: a.UserID, Name: name, Prefix: raw[:10], Hash: hash, ExpiresAt: expiry(req.ExpireDays)})
	if err != nil {
		return nil, err
	}
	return &Issued{ID: id, Token: raw, Name: name}, nil
}

func (s *Service) RevokePersonal(ctx context.Context, a principal.Actor, id uuid.UUID) error {
	if a.ViaPAT {
		return ErrNeedPassword
	}
	return s.repo.DeletePersonal(ctx, id, a.UserID)
}

// AuthenticatePersonal resolves a personal access token to the user it acts as.
func (s *Service) AuthenticatePersonal(ctx context.Context, raw string) (principal.Actor, error) {
	rec, err := s.repo.PersonalByHash(ctx, tokenutil.Hash(raw))
	if err != nil {
		return principal.Actor{}, ErrInvalidToken
	}
	email, err := s.pii.Open("users.email", rec.EmailEnc)
	if err != nil || (rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now())) {
		return principal.Actor{}, ErrInvalidToken
	}
	go s.repo.TouchPersonal(context.Background(), rec.ID)
	return principal.Actor{UserID: rec.UserID, Email: email, ViaPAT: true}, nil
}

// AuthenticateService resolves a service token and enforces expiry and its IP allowlist.
func (s *Service) AuthenticateService(ctx context.Context, raw, ip string) (principal.Actor, error) {
	if !strings.HasPrefix(raw, tokenutil.PrefixService) {
		return principal.Actor{}, ErrNeedService
	}
	t, err := s.repo.ServiceByHash(ctx, tokenutil.Hash(raw))
	if err != nil || (t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now())) {
		return principal.Actor{}, ErrInvalidToken
	}
	st := &principal.ServiceToken{ID: t.ID, ProjectID: t.ProjectID, OrgID: t.OrgID, EnvID: t.EnvID, EnvSlug: t.EnvSlug, Name: t.Name, PathPrefix: t.PathPrefix, CanWrite: t.CanWrite}
	actor := principal.Actor{Token: st, IP: ip}
	if !ipAllowed(t.CIDRs, ip) {
		s.audit.Record(ctx, actor, t.OrgID, &t.ProjectID, "token.denied", t.Name, map[string]any{"reason": "ip_not_allowed"})
		return principal.Actor{}, ErrIPNotAllowed
	}
	go s.repo.TouchService(context.Background(), t.ID)
	return actor, nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.TrimSpace(s))
	}
	return out
}
