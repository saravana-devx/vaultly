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
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

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

func (s *Service) EnvID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	return s.repo.EnvID(ctx, projectID, slug)
}

type Repo struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

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
	err := r.db.QueryRow(ctx, `SELECT role FROM member_env_roles WHERE org_id=$1 AND user_id=$2 AND env_slug=$3`, orgID, userID, slug).Scan(&role)
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
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) EnvID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT id FROM environments WHERE project_id=$1 AND slug=$2`, projectID, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, apperr.NotFound("environment not found")
	}
	return id, err
}

var _ Repository = (*Repo)(nil)
