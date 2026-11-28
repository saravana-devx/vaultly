package access

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"vaultly/internal/apperr"
)

var ctx = context.Background()

// fakeRepo holds one user's workspace role and per-environment overrides.
type fakeRepo struct {
	orgRole   string
	member    bool
	overrides map[string]string // env slug -> role
	envs      []Env
	protected map[uuid.UUID]bool
}

func (f *fakeRepo) EnvProtected(_ context.Context, id uuid.UUID) (bool, error) {
	return f.protected[id], nil
}

func (f *fakeRepo) OrgRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	if !f.member {
		return "", ErrNotFound
	}
	return f.orgRole, nil
}

func (f *fakeRepo) ProjectWithRole(_ context.Context, _, projectID uuid.UUID) (*Project, error) {
	if !f.member {
		return nil, ErrNotFound
	}
	return &Project{ID: projectID, OrgID: uuid.New(), Name: "sealed", Role: f.orgRole}, nil
}

func (f *fakeRepo) EnvOverride(_ context.Context, _, _ uuid.UUID, slug string) (string, bool, error) {
	r, ok := f.overrides[slug]
	return r, ok, nil
}

func (f *fakeRepo) Envs(context.Context, uuid.UUID) ([]Env, error) { return f.envs, nil }

func (f *fakeRepo) EnvID(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return uuid.New(), nil
}

type plainNamer struct{}

func (plainNamer) OpenName(_, s string) string { return "opened:" + s }

func newSvc(f *fakeRepo) *Service { return NewService(f, plainNamer{}) }

func isKind(err error, k apperr.Kind) bool {
	ae, ok := apperr.As(err)
	return ok && ae.Kind == k
}

func TestOrgRoleRanking(t *testing.T) {
	cases := []struct {
		have, need string
		ok         bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleViewer, true},
		{RoleDeveloper, RoleDeveloper, true},
		{RoleDeveloper, RoleAdmin, false},
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleDeveloper, false},
		{"bogus", RoleViewer, false}, // unknown roles rank as none
	}
	for _, c := range cases {
		_, err := newSvc(&fakeRepo{member: true, orgRole: c.have}).Org(ctx, uuid.New(), uuid.New(), c.need)
		if (err == nil) != c.ok {
			t.Errorf("have %s need %s: got err=%v, want ok=%v", c.have, c.need, err, c.ok)
		}
		if err != nil && !isKind(err, apperr.KindForbidden) {
			t.Errorf("have %s need %s: want forbidden, got %v", c.have, c.need, err)
		}
	}
}

func TestNonMemberIsNotFound(t *testing.T) {
	// Non-members must get not-found, not forbidden, so workspace and project IDs don't leak.
	s := newSvc(&fakeRepo{member: false})
	if _, err := s.Org(ctx, uuid.New(), uuid.New(), RoleViewer); !isKind(err, apperr.KindNotFound) {
		t.Fatalf("Org: want not found, got %v", err)
	}
	if _, err := s.Project(ctx, uuid.New(), uuid.New(), RoleViewer); !isKind(err, apperr.KindNotFound) {
		t.Fatalf("Project: want not found, got %v", err)
	}
}

func TestProjectOpensSealedName(t *testing.T) {
	p, err := newSvc(&fakeRepo{member: true, orgRole: RoleViewer}).Project(ctx, uuid.New(), uuid.New(), RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "opened:sealed" {
		t.Fatalf("name not opened: %q", p.Name)
	}
}

func TestEnvRoleOverrides(t *testing.T) {
	cases := []struct {
		name      string
		orgRole   string
		overrides map[string]string
		env       string
		want      string
	}{
		{"no override falls back to workspace role", RoleDeveloper, nil, "prod", RoleDeveloper},
		{"override can lower access", RoleDeveloper, map[string]string{"prod": RoleViewer}, "prod", RoleViewer},
		{"override can remove access", RoleDeveloper, map[string]string{"prod": RoleNone}, "prod", RoleNone},
		{"override can raise access", RoleViewer, map[string]string{"dev": RoleDeveloper}, "dev", RoleDeveloper},
		{"override only applies to its env", RoleDeveloper, map[string]string{"prod": RoleNone}, "dev", RoleDeveloper},
		{"admins ignore overrides", RoleAdmin, map[string]string{"prod": RoleNone}, "prod", RoleAdmin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSvc(&fakeRepo{member: true, orgRole: c.orgRole, overrides: c.overrides})
			p := &Project{ID: uuid.New(), OrgID: uuid.New(), Role: c.orgRole}
			got, err := s.EnvRole(ctx, p, uuid.New(), c.env)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestNeedEnv(t *testing.T) {
	s := newSvc(&fakeRepo{member: true, orgRole: RoleDeveloper, overrides: map[string]string{"prod": RoleViewer}})
	p := &Project{ID: uuid.New(), OrgID: uuid.New(), Role: RoleDeveloper}
	if err := s.NeedEnv(ctx, p, uuid.New(), "prod", RoleViewer); err != nil {
		t.Fatalf("viewer read on prod should pass: %v", err)
	}
	if err := s.NeedEnv(ctx, p, uuid.New(), "prod", RoleDeveloper); !isKind(err, apperr.KindForbidden) {
		t.Fatalf("developer write on prod should be forbidden, got %v", err)
	}
	if err := s.NeedEnv(ctx, p, uuid.New(), "dev", RoleDeveloper); err != nil {
		t.Fatalf("developer write on dev should pass: %v", err)
	}
}

func TestReadableEnvsHidesNoneOverrides(t *testing.T) {
	envs := []Env{{ID: uuid.New(), Slug: "dev"}, {ID: uuid.New(), Slug: "staging"}, {ID: uuid.New(), Slug: "prod"}}
	s := newSvc(&fakeRepo{member: true, orgRole: RoleDeveloper, envs: envs, overrides: map[string]string{"prod": RoleNone}})
	p := &Project{ID: uuid.New(), OrgID: uuid.New(), Role: RoleDeveloper}
	got, err := s.ReadableEnvs(ctx, p, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["dev"] != envs[0].ID || got["staging"] != envs[1].ID {
		t.Fatalf("unexpected readable envs: %v", got)
	}
	if _, ok := got["prod"]; ok {
		t.Fatal("prod is overridden to none and must not be readable")
	}
}

func TestValidRoles(t *testing.T) {
	if ValidWorkspaceRole(RoleNone) {
		t.Error("none is not a workspace role")
	}
	if ValidEnvRole(RoleAdmin) {
		t.Error("admin cannot be granted per environment")
	}
	for _, r := range []string{RoleAdmin, RoleDeveloper, RoleViewer} {
		if !ValidWorkspaceRole(r) {
			t.Errorf("%s should be a valid workspace role", r)
		}
	}
	for _, r := range []string{RoleNone, RoleViewer, RoleDeveloper} {
		if !ValidEnvRole(r) {
			t.Errorf("%s should be a valid env role", r)
		}
	}
}

func TestNeedsApproval(t *testing.T) {
	prod, dev := uuid.New(), uuid.New()
	for _, c := range []struct {
		role string
		env  uuid.UUID
		want bool
	}{
		{RoleDeveloper, prod, true},
		{RoleDeveloper, dev, false},
		{RoleViewer, prod, true},
		{RoleAdmin, prod, false}, // admins write directly, as they can already change protection
	} {
		s := newSvc(&fakeRepo{member: true, orgRole: c.role, protected: map[uuid.UUID]bool{prod: true}})
		got, err := s.NeedsApproval(ctx, &Project{Role: c.role}, c.env)
		if err != nil || got != c.want {
			t.Errorf("%s on %v: got %v, %v; want %v", c.role, c.env == prod, got, err, c.want)
		}
	}
}
