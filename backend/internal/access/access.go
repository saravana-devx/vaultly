// Package access answers "may this user do this here?". The workspace role gives the default;
// a per-environment override (none | viewer | developer) wins for that environment name.
package access

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/apperr"
)

const (
	RoleAdmin     = "admin"
	RoleDeveloper = "developer"
	RoleViewer    = "viewer"
	RoleNone      = "none"
)

var rank = map[string]int{RoleNone: 0, RoleViewer: 1, RoleDeveloper: 2, RoleAdmin: 3}

func Rank(role string) int { return rank[role] }

func ValidWorkspaceRole(r string) bool {
	return r == RoleAdmin || r == RoleDeveloper || r == RoleViewer
}

func ValidEnvRole(r string) bool { return r == RoleNone || r == RoleViewer || r == RoleDeveloper }

var ErrNotFound = apperr.NotFound("not found")

func forbidden(need string) error { return apperr.Forbidden("requires " + need + " role") }

type Project struct {
	ID    uuid.UUID
	OrgID uuid.UUID
	Name  string
	Role  string
}

type Env struct {
	ID   uuid.UUID
	Name string
	Slug string
}

type Repository interface {
	OrgRole(ctx context.Context, userID, orgID uuid.UUID) (string, error)
	ProjectWithRole(ctx context.Context, userID, projectID uuid.UUID) (*Project, error)
	EnvOverride(ctx context.Context, orgID, userID uuid.UUID, slug string) (role string, found bool, err error)
	Envs(ctx context.Context, projectID uuid.UUID) ([]Env, error)
	EnvID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error)
	EnvProtected(ctx context.Context, envID uuid.UUID) (bool, error)
}

// Namer opens names that are stored encrypted.
type Namer interface {
	OpenName(field, s string) string
}

type Service struct {
	repo  Repository
	namer Namer
}

func NewService(repo Repository, namer Namer) *Service { return &Service{repo: repo, namer: namer} }

// Org returns the caller's workspace role, or a not-found error if they are not a member.
func (s *Service) Org(ctx context.Context, userID, orgID uuid.UUID, need string) (string, error) {
	role, err := s.repo.OrgRole(ctx, userID, orgID)
	if err != nil {
		return "", err
	}
	if rank[role] < rank[need] {
		return "", forbidden(need)
	}
	return role, nil
}

func (s *Service) Project(ctx context.Context, userID, projectID uuid.UUID, need string) (*Project, error) {
	p, err := s.repo.ProjectWithRole(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	if rank[p.Role] < rank[need] {
		return nil, forbidden(need)
	}
	p.Name = s.namer.OpenName("projects.name", p.Name)
	return p, nil
}

func (s *Service) EnvRole(ctx context.Context, p *Project, userID uuid.UUID, slug string) (string, error) {
	if p.Role == RoleAdmin {
		return RoleAdmin, nil
	}
	role, found, err := s.repo.EnvOverride(ctx, p.OrgID, userID, slug)
	if err != nil {
		return "", err
	}
	if found {
		return role, nil
	}
	return p.Role, nil
}

func (s *Service) NeedEnv(ctx context.Context, p *Project, userID uuid.UUID, slug, need string) error {
	role, err := s.EnvRole(ctx, p, userID, slug)
	if err != nil {
		return err
	}
	if rank[role] >= rank[need] {
		return nil
	}
	return apperr.Forbidden(fmt.Sprintf("you need %s access to the %s environment", need, slug))
}

// ReadableEnvs lists the environments (slug -> id) the user may at least read.
func (s *Service) ReadableEnvs(ctx context.Context, p *Project, userID uuid.UUID) (map[string]uuid.UUID, error) {
	envs, err := s.repo.Envs(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]uuid.UUID{}
	for _, e := range envs {
		role, err := s.EnvRole(ctx, p, userID, e.Slug)
		if err != nil {
			return nil, err
		}
		if rank[role] >= rank[RoleViewer] {
			out[e.Slug] = e.ID
		}
	}
	return out, nil
}

// Envs lists a project's environments in display order (callers check access themselves).
func (s *Service) Envs(ctx context.Context, projectID uuid.UUID) ([]Env, error) {
	return s.repo.Envs(ctx, projectID)
}

func (s *Service) EnvID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	return s.repo.EnvID(ctx, projectID, slug)
}

// NeedsApproval reports whether a change by this caller to env must go through a change request:
// the environment is protected and the caller is not a workspace admin.
func (s *Service) NeedsApproval(ctx context.Context, p *Project, envID uuid.UUID) (bool, error) {
	if p.Role == RoleAdmin {
		return false, nil
	}
	return s.repo.EnvProtected(ctx, envID)
}

// Environment names and slugs are stored sealed; slug_hash is what lookups compare.
type Sealer interface {
	Namer
	Hash(field, s string) []byte
}

type Repo struct {
	db  *pgxpool.Pool
	box Sealer
}

func NewRepository(db *pgxpool.Pool, box Sealer) *Repo { return &Repo{db: db, box: box} }

func (r *Repo) OrgRole(ctx context.Context, userID, orgID uuid.UUID) (string, error) {
	var role string
	err := r.db.QueryRow(ctx, `SELECT role FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

func (r *Repo) ProjectWithRole(ctx context.Context, userID, projectID uuid.UUID) (*Project, error) {
	p := &Project{}
	err := r.db.QueryRow(ctx, `
		SELECT p.id, p.org_id, p.name, m.role FROM projects p
		JOIN memberships m ON m.org_id = p.org_id AND m.user_id = $2
		WHERE p.id = $1`, projectID, userID).Scan(&p.ID, &p.OrgID, &p.Name, &p.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (r *Repo) EnvOverride(ctx context.Context, orgID, userID uuid.UUID, slug string) (string, bool, error) {
	var role string
	err := r.db.QueryRow(ctx, `SELECT role FROM member_env_roles WHERE org_id=$1 AND user_id=$2 AND env_slug_hash=$3`, orgID, userID, r.box.Hash("environments.slug", slug)).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return role, err == nil, err
}

func (r *Repo) Envs(ctx context.Context, projectID uuid.UUID) ([]Env, error) {
	rows, err := r.db.Query(ctx, `SELECT id,name,slug FROM environments WHERE project_id=$1 ORDER BY position, name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Env
	for rows.Next() {
		var e Env
		if err := rows.Scan(&e.ID, &e.Name, &e.Slug); err != nil {
			return nil, err
		}
		e.Name, e.Slug = r.box.OpenName("environments.name", e.Name), r.box.OpenName("environments.slug", e.Slug)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) EnvID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT id FROM environments WHERE project_id=$1 AND slug_hash=$2`, projectID, r.box.Hash("environments.slug", slug)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, apperr.NotFound("environment not found")
	}
	return id, err
}

func (r *Repo) EnvProtected(ctx context.Context, envID uuid.UUID) (bool, error) {
	var on bool
	err := r.db.QueryRow(ctx, `SELECT protected FROM environments WHERE id=$1`, envID).Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperr.NotFound("environment not found")
	}
	return on, err
}

var _ Repository = (*Repo)(nil)
