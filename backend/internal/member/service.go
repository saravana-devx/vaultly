package member

import (
	"context"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/principal"
	"vaultly/internal/slug"
	"vaultly/internal/user"
)

type MemberRepository interface {
	List(ctx context.Context, org uuid.UUID) ([]Row, error)
	EnvOverrides(ctx context.Context, org uuid.UUID) (map[uuid.UUID]map[string]string, error)
	Add(ctx context.Context, org, user uuid.UUID, role string) error
	SetRole(ctx context.Context, org, user uuid.UUID, role string) error
	Remove(ctx context.Context, org, user uuid.UUID) error
	AdminInfo(ctx context.Context, org, user uuid.UUID) (admins int, isAdmin bool, err error)
	EnvRoles(ctx context.Context, org, user uuid.UUID) (map[string]string, error)
	ReplaceEnvRoles(ctx context.Context, org, user uuid.UUID, roles map[string]string) error
}

type Service struct {
	repo   MemberRepository
	access *access.Service
	users  *user.Service
	audit  *audit.Service
}

func NewService(repo MemberRepository, access *access.Service, users *user.Service, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, users: users, audit: audit}
}

func (s *Service) List(ctx context.Context, a principal.Actor, org uuid.UUID) ([]Member, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleViewer); err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, org)
	if err != nil {
		return nil, err
	}
	overrides, err := s.repo.EnvOverrides(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(rows))
	for _, row := range rows {
		email, name, err := s.users.OpenFields(row.EmailEnc, row.NameEnc)
		if err != nil {
			continue
		}
		roles := overrides[row.ID]
		if roles == nil {
			roles = map[string]string{}
		}
		out = append(out, Member{ID: row.ID, Email: email, Name: name, Role: row.Role, JoinedAt: row.JoinedAt, EnvRoles: roles})
	}
	return out, nil
}

// Add puts an existing account into the workspace (people without an account use invitations).
func (s *Service) Add(ctx context.Context, a principal.Actor, org uuid.UUID, req *AddMemberRequest) error {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return err
	}
	uid, err := s.users.IDByEmail(ctx, user.NormEmail(req.Email))
	if err != nil {
		return err
	}
	if err := s.repo.Add(ctx, org, uid, req.Role); err != nil {
		return err
	}
	s.audit.Record(ctx, a, org, nil, "member.add", req.Email, map[string]any{"role": req.Role})
	return nil
}

func (s *Service) UpdateRole(ctx context.Context, a principal.Actor, org, target uuid.UUID, req *UpdateRoleRequest) error {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return err
	}
	if req.Role != access.RoleAdmin {
		if err := s.guardLastAdmin(ctx, org, target); err != nil {
			return err
		}
	}
	if err := s.repo.SetRole(ctx, org, target, req.Role); err != nil {
		return err
	}
	s.audit.Record(ctx, a, org, nil, "member.role_change", s.emailOf(ctx, target), map[string]any{"role": req.Role})
	return nil
}

// Remove lets an admin remove anyone and lets anyone leave.
func (s *Service) Remove(ctx context.Context, a principal.Actor, org, target uuid.UUID) error {
	need := access.RoleAdmin
	if target == a.UserID {
		need = access.RoleViewer
	}
	if _, err := s.access.Org(ctx, a.UserID, org, need); err != nil {
		return err
	}
	if err := s.guardLastAdmin(ctx, org, target); err != nil {
		return err
	}
	email := s.emailOf(ctx, target)
	if err := s.repo.Remove(ctx, org, target); err != nil {
		return err
	}
	s.audit.Record(ctx, a, org, nil, "member.remove", email, nil)
	return nil
}

func (s *Service) EnvRoles(ctx context.Context, a principal.Actor, org, target uuid.UUID) (map[string]string, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	return s.repo.EnvRoles(ctx, org, target)
}

// SetEnvRoles replaces a member's overrides; "inherit" (or a missing name) falls back to their workspace role.
func (s *Service) SetEnvRoles(ctx context.Context, a principal.Actor, org, target uuid.UUID, req *EnvRolesRequest) error {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return err
	}
	keep := map[string]string{}
	for name, role := range req.Roles {
		if slug.Make(name) != name || (role != "inherit" && !access.ValidEnvRole(role)) {
			return ErrInvalidEnvRole
		}
		if role != "inherit" {
			keep[name] = role
		}
	}
	if err := s.repo.ReplaceEnvRoles(ctx, org, target, keep); err != nil {
		return err
	}
	s.audit.Record(ctx, a, org, nil, "member.env_roles", s.emailOf(ctx, target), map[string]any{"roles": req.Roles})
	return nil
}

func (s *Service) guardLastAdmin(ctx context.Context, org, target uuid.UUID) error {
	admins, isAdmin, err := s.repo.AdminInfo(ctx, org, target)
	if err != nil {
		return err
	}
	if isAdmin && admins <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func (s *Service) emailOf(ctx context.Context, id uuid.UUID) string {
	p, err := s.users.Profile(ctx, id)
	if err != nil {
		return id.String()
	}
	return p.Email
}
