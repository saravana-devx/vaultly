package project

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/keystore"
	"vaultly/internal/principal"
	"vaultly/internal/slug"
)

type ProjectRepository interface {
	ListByOrg(ctx context.Context, org uuid.UUID) ([]Summary, error)
	EnvsFor(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID][]Env, error)
	Create(ctx context.Context, org uuid.UUID, name string, wrappedKey []byte) (uuid.UUID, error)
	Delete(ctx context.Context, id uuid.UUID) error
	CreateEnv(ctx context.Context, project uuid.UUID, name, slug string) (Env, error)
	DeleteEnv(ctx context.Context, project, env uuid.UUID) (string, error)
}

// LeaseRevoker drops live dynamic-secret credentials before their owner is deleted.
type LeaseRevoker interface {
	RevokeForProject(ctx context.Context, projectID uuid.UUID)
	RevokeForEnv(ctx context.Context, envID uuid.UUID)
}

type Service struct {
	repo   ProjectRepository
	access *access.Service
	keys   *keystore.Service
	leases LeaseRevoker
	audit  *audit.Service
}

func NewService(repo ProjectRepository, access *access.Service, keys *keystore.Service, leases LeaseRevoker, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, keys: keys, leases: leases, audit: audit}
}

func (s *Service) List(ctx context.Context, a principal.Actor, org uuid.UUID) ([]Summary, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleViewer); err != nil {
		return nil, err
	}
	projects, err := s.repo.ListByOrg(ctx, org)
	if err != nil || len(projects) == 0 {
		return projects, err
	}
	ids := make([]uuid.UUID, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	envs, err := s.repo.EnvsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if e := envs[projects[i].ID]; e != nil {
			projects[i].Environments = e
		}
	}
	return projects, nil
}

func (s *Service) Create(ctx context.Context, a principal.Actor, org uuid.UUID, req *CreateProjectRequest) (*Created, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	_, wrapped, err := s.keys.NewProjectKey(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.repo.Create(ctx, org, name, wrapped)
	if err != nil {
		return nil, err
	}
	envs, err := s.repo.EnvsFor(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, org, &id, "project.create", name, nil)
	return &Created{ID: id, Name: name, Environments: envs[id]}, nil
}

// Get hides environments the caller has no access to.
func (s *Service) Get(ctx context.Context, a principal.Actor, id uuid.UUID) (*Detail, error) {
	p, err := s.access.Project(ctx, a.UserID, id, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	envsByProject, err := s.repo.EnvsFor(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	envs := envsByProject[id]
	if p.Role != access.RoleAdmin {
		visible := make([]Env, 0, len(envs))
		for _, e := range envs {
			role, err := s.access.EnvRole(ctx, p, a.UserID, e.Slug)
			if err != nil {
				return nil, err
			}
			if access.Rank(role) >= access.Rank(access.RoleViewer) {
				visible = append(visible, e)
			}
		}
		envs = visible
	}
	if envs == nil {
		envs = []Env{}
	}
	return &Detail{ID: p.ID, OrgID: p.OrgID, Name: p.Name, Role: p.Role, Environments: envs}, nil
}

func (s *Service) Delete(ctx context.Context, a principal.Actor, id uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, id, access.RoleAdmin)
	if err != nil {
		return err
	}
	s.leases.RevokeForProject(ctx, id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.keys.Forget(id)
	s.audit.Record(ctx, a, p.OrgID, &id, "project.delete", p.Name, nil)
	return nil
}

func (s *Service) CreateEnv(ctx context.Context, a principal.Actor, id uuid.UUID, req *CreateEnvRequest) (*Env, error) {
	p, err := s.access.Project(ctx, a.UserID, id, access.RoleAdmin)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	envSlug := slug.Make(req.Slug)
	if envSlug == "" {
		envSlug = slug.Make(name)
	}
	if envSlug == "" || len(envSlug) > 32 {
		return nil, ErrInvalidEnvSlug
	}
	env, err := s.repo.CreateEnv(ctx, id, name, envSlug)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, p.OrgID, &id, "environment.create", env.Slug, nil)
	return &env, nil
}

func (s *Service) DeleteEnv(ctx context.Context, a principal.Actor, id, env uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, id, access.RoleAdmin)
	if err != nil {
		return err
	}
	s.leases.RevokeForEnv(ctx, env)
	envSlug, err := s.repo.DeleteEnv(ctx, id, env)
	if err != nil {
		return err
	}
	s.audit.Record(ctx, a, p.OrgID, &id, "environment.delete", envSlug, nil)
	return nil
}
