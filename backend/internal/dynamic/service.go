package dynamic

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/apperr"
	"vaultly/internal/audit"
	"vaultly/internal/crypto"
	"vaultly/internal/keystore"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
	"vaultly/internal/slug"
	"vaultly/internal/tokenutil"
)

type DynamicRepository interface {
	List(ctx context.Context, projectID uuid.UUID) ([]ConfigRow, error)
	Create(ctx context.Context, n NewConfig) (uuid.UUID, error)
	Name(ctx context.Context, id, projectID uuid.UUID) (string, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ConfigForLease(ctx context.Context, id, projectID uuid.UUID) (*ConfigRow, error)
	IDByName(ctx context.Context, projectID, envID uuid.UUID, name string) (uuid.UUID, error)
	InsertLease(ctx context.Context, configID uuid.UUID, username string, exp time.Time, createdBy string) (uuid.UUID, error)
	Leases(ctx context.Context, configID, projectID uuid.UUID) ([]LeaseView, error)
	ActiveLease(ctx context.Context, leaseID uuid.UUID) (*LeaseTarget, error)
	MarkRevoked(ctx context.Context, leaseID uuid.UUID)
	MarkRevokeFailed(ctx context.Context, leaseID uuid.UUID, msg string)
	LeaseIDsForConfig(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	LeaseIDsForProject(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	LeaseIDsForEnv(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	ExpiredLeaseIDs(ctx context.Context, limit int) ([]uuid.UUID, error)
}

type Service struct {
	repo     DynamicRepository
	access   *access.Service
	keys     *keystore.Service
	pii      *pii.Box
	provider Provider
	audit    *audit.Service
}

func NewService(repo DynamicRepository, access *access.Service, keys *keystore.Service, box *pii.Box, provider Provider, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, keys: keys, pii: box, provider: provider, audit: audit}
}

func (s *Service) sealConfig(projectID uuid.UUID, dek []byte, cfg Config) ([]byte, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return crypto.Encrypt(dek, b, []byte(projectID.String()))
}

func (s *Service) openConfig(projectID uuid.UUID, dek, enc []byte) (cfg Config, err error) {
	b, err := crypto.Decrypt(dek, enc, []byte(projectID.String()))
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(b, &cfg)
}

func hostInfo(adminURL string) map[string]any {
	u, err := url.Parse(adminURL)
	if err != nil {
		return map[string]any{}
	}
	return map[string]any{"host": u.Host, "database": strings.TrimPrefix(u.Path, "/"), "admin_user": u.User.Username()}
}

func (s *Service) List(ctx context.Context, a principal.Actor, projectID uuid.UUID) ([]View, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		cfg, _ := s.openConfig(projectID, dek, r.ConfigEnc)
		out = append(out, View{ID: r.ID, Name: r.Name, Provider: r.Provider, Env: r.Env, Target: hostInfo(cfg.AdminURL), Grants: cfg.Grants,
			DefaultTTL: r.DefaultTTL, MaxTTL: r.MaxTTL, ActiveLeases: r.Active})
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, a principal.Actor, projectID uuid.UUID, req *CreateRequest) (*Created, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return nil, err
	}
	name := slug.Make(req.Name)
	if name == "" {
		return nil, ErrInvalidName
	}
	def, max := req.DefaultTTL, req.MaxTTL
	if def <= 0 {
		def = 3600
	}
	if max <= 0 {
		max = 86400
	}
	if def < 60 || max < def || max > 30*86400 {
		return nil, ErrInvalidTTL
	}
	envID, err := s.access.EnvID(ctx, projectID, req.Env)
	if err != nil {
		return nil, ErrEnvNotFound
	}
	cfg := Config{AdminURL: strings.TrimSpace(req.AdminURL), Grants: req.Grants}
	vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.provider.Validate(vctx, cfg); err != nil {
		return nil, apperr.Invalid(err.Error())
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	enc, err := s.sealConfig(projectID, dek, cfg)
	if err != nil {
		return nil, err
	}
	id, err := s.repo.Create(ctx, NewConfig{ProjectID: projectID, EnvID: envID, Name: name, ConfigEnc: enc, DefaultTTL: def, MaxTTL: max})
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "dynamic.create", req.Name, map[string]any{"env": req.Env})
	return &Created{ID: id}, nil
}

func (s *Service) Delete(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return err
	}
	name, err := s.repo.Name(ctx, id, projectID)
	if err != nil {
		return err
	}
	s.revokeAll(ctx, s.repo.LeaseIDsForConfig, id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "dynamic.delete", name, nil)
	return nil
}

// Lease issues credentials for a user with developer access to the environment.
func (s *Service) Lease(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, req *LeaseRequest) (*Lease, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	cfgRow, err := s.repo.ConfigForLease(ctx, id, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.access.NeedEnv(ctx, p, a.UserID, cfgRow.Env, access.RoleDeveloper); err != nil {
		return nil, err
	}
	l, err := s.issue(ctx, cfgRow, req.TTL, a.Email)
	if err != nil {
		return nil, apperr.Upstream("could not create credentials: " + err.Error())
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "dynamic.lease", cfgRow.Name, map[string]any{"env": cfgRow.Env, "username": l.Username, "expires_at": l.ExpiresAt})
	return l, nil
}

// TokenLease lets a service token request credentials for a dynamic secret in its own environment.
func (s *Service) TokenLease(ctx context.Context, a principal.Actor, name string, req *LeaseRequest) (*Lease, error) {
	t := a.Token
	id, err := s.repo.IDByName(ctx, t.ProjectID, t.EnvID, name)
	if err != nil {
		return nil, err
	}
	cfgRow, err := s.repo.ConfigForLease(ctx, id, t.ProjectID)
	if err != nil {
		return nil, err
	}
	l, err := s.issue(ctx, cfgRow, req.TTL, a.Label())
	if err != nil {
		return nil, apperr.Upstream("could not create credentials")
	}
	s.audit.Record(ctx, a, t.OrgID, &t.ProjectID, "dynamic.lease", cfgRow.Name, map[string]any{"env": t.EnvSlug, "username": l.Username, "expires_at": l.ExpiresAt})
	return l, nil
}

func (s *Service) issue(ctx context.Context, row *ConfigRow, ttl int, createdBy string) (*Lease, error) {
	if ttl <= 0 {
		ttl = row.DefaultTTL
	}
	ttl = max(60, min(row.MaxTTL, ttl))
	dek, err := s.keys.ProjectKey(ctx, row.ProjectID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.openConfig(row.ProjectID, dek, row.ConfigEnc)
	if err != nil {
		return nil, err
	}
	suffix, err := tokenutil.RandomLower(16)
	if err != nil {
		return nil, err
	}
	password, err := tokenutil.RandomString(32)
	if err != nil {
		return nil, err
	}
	username := "v_" + suffix
	exp := time.Now().Add(time.Duration(ttl) * time.Second).UTC()
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.provider.Create(cctx, cfg, username, password, exp); err != nil {
		return nil, err
	}
	connURL, _ := s.provider.ConnectionURL(cfg, username, password)
	id, err := s.repo.InsertLease(ctx, row.ID, username, exp, s.pii.Label(createdBy))
	if err != nil {
		_ = s.provider.Revoke(ctx, cfg, username)
		return nil, err
	}
	return &Lease{ID: id, Username: username, Password: password, URL: connURL, ExpiresAt: exp}, nil
}

func (s *Service) Leases(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) ([]LeaseView, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	out, err := s.repo.Leases(ctx, id, projectID)
	for i := range out {
		l := &out[i]
		l.CreatedBy = s.pii.OpenLabel(l.CreatedBy)
		switch {
		case l.RevokedAt != nil:
			l.State = "revoked"
		case l.ExpiresAt.Before(time.Now()):
			l.State = "expired"
		default:
			l.State = "active"
		}
	}
	return out, err
}

func (s *Service) Revoke(ctx context.Context, a principal.Actor, projectID, leaseID uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return err
	}
	if err := s.revokeOne(ctx, leaseID, projectID); err != nil {
		if _, ok := apperr.As(err); ok {
			return err
		}
		return apperr.Upstream(err.Error())
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "dynamic.revoke", leaseID.String(), nil)
	return nil
}

// revokeOne drops one lease's credentials. projectID uuid.Nil skips the ownership check (system jobs);
// with a project, a lease that is not live in it is reported as not found instead of "revoked".
func (s *Service) revokeOne(ctx context.Context, leaseID, projectID uuid.UUID) error {
	t, err := s.repo.ActiveLease(ctx, leaseID)
	if err != nil {
		return err
	}
	if t == nil || (projectID != uuid.Nil && t.ProjectID != projectID) {
		if projectID != uuid.Nil {
			return ErrNotFound
		}
		return nil
	}
	dek, err := s.keys.ProjectKey(ctx, t.ProjectID)
	if err != nil {
		return err
	}
	cfg, err := s.openConfig(t.ProjectID, dek, t.ConfigEnc)
	if err != nil {
		return err
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.provider.Revoke(rctx, cfg, t.Username); err != nil {
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		s.repo.MarkRevokeFailed(ctx, leaseID, msg)
		return err
	}
	s.repo.MarkRevoked(ctx, leaseID)
	return nil
}

func (s *Service) revokeAll(ctx context.Context, ids func(context.Context, uuid.UUID) ([]uuid.UUID, error), arg uuid.UUID) {
	list, err := ids(ctx, arg)
	if err != nil {
		return
	}
	for _, id := range list {
		_ = s.revokeOne(ctx, id, uuid.Nil)
	}
}

// RevokeForProject and RevokeForEnv make sure deleting a project or environment never leaves
// live database roles behind.
func (s *Service) RevokeForProject(ctx context.Context, projectID uuid.UUID) {
	s.revokeAll(ctx, s.repo.LeaseIDsForProject, projectID)
}

func (s *Service) RevokeForEnv(ctx context.Context, envID uuid.UUID) {
	s.revokeAll(ctx, s.repo.LeaseIDsForEnv, envID)
}

// RevokeExpired is called by the janitor and returns how many leases it processed.
func (s *Service) RevokeExpired(ctx context.Context) int {
	ids, err := s.repo.ExpiredLeaseIDs(ctx, 50)
	if err != nil {
		return 0
	}
	for _, id := range ids {
		_ = s.revokeOne(ctx, id, uuid.Nil)
	}
	return len(ids)
}
