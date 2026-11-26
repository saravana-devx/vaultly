package org

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/pii"
)

type OrgRepository interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]Org, error)
	Create(ctx context.Context, userID uuid.UUID, name string) (uuid.UUID, error)
	Counts(ctx context.Context, org uuid.UUID) (Counts, error)
	Activity(ctx context.Context, org uuid.UUID) ([]DayCount, error)
	Recent(ctx context.Context, org uuid.UUID, limit int) ([]RecentEvent, error)
	PendingInvites(ctx context.Context, org uuid.UUID) (int, error)
	TokenExpiries(ctx context.Context, org uuid.UUID) ([]ExpiryRow, error)
	SecretExpiries(ctx context.Context, org uuid.UUID) ([]ExpiryRow, error)
	EnvSlugs(ctx context.Context, org uuid.UUID) ([]string, error)
}

type Service struct {
	repo   OrgRepository
	access *access.Service
	pii    *pii.Box
}

func NewService(repo OrgRepository, access *access.Service, box *pii.Box) *Service {
	return &Service{repo: repo, access: access, pii: box}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Org, error) {
	orgs, err := s.repo.ListForUser(ctx, userID)
	for i := range orgs {
		orgs[i].Name = s.pii.OpenName("orgs.name", orgs[i].Name)
	}
	return orgs, err
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req *CreateOrgRequest) (*Org, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	id, err := s.repo.Create(ctx, userID, s.pii.SealName("orgs.name", name))
	if err != nil {
		return nil, err
	}
	return &Org{ID: id, Name: name, Role: access.RoleAdmin}, nil
}

func (s *Service) EnvSlugs(ctx context.Context, userID, org uuid.UUID) ([]string, error) {
	if _, err := s.access.Org(ctx, userID, org, access.RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.EnvSlugs(ctx, org)
}

// Stats feeds the dashboard. Audit data and the token inventory are admin-only.
func (s *Service) Stats(ctx context.Context, userID, org uuid.UUID) (*Stats, error) {
	role, err := s.access.Org(ctx, userID, org, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.Counts(ctx, org)
	if err != nil {
		return nil, err
	}
	activity, err := s.repo.Activity(ctx, org)
	if err != nil {
		return nil, err
	}
	out := &Stats{Counts: counts, Activity: activity}
	if role != access.RoleAdmin {
		return out, nil
	}

	if out.Recent, err = s.repo.Recent(ctx, org, 8); err != nil {
		return nil, err
	}
	for i := range out.Recent {
		out.Recent[i].Actor = s.pii.OpenLabel(out.Recent[i].Actor)
		out.Recent[i].Resource = s.pii.OpenLabel(out.Recent[i].Resource)
		if out.Recent[i].Project != nil {
			name := s.pii.OpenName("projects.name", *out.Recent[i].Project)
			out.Recent[i].Project = &name
		}
	}
	pending, err := s.repo.PendingInvites(ctx, org)
	if err != nil {
		return nil, err
	}
	out.PendingInvites = &pending

	tokens, err := s.repo.TokenExpiries(ctx, org)
	if err != nil {
		return nil, err
	}
	secrets, err := s.repo.SecretExpiries(ctx, org)
	if err != nil {
		return nil, err
	}
	out.Attention = []Attention{}
	for i := range tokens {
		tokens[i].Project = s.pii.OpenName("projects.name", tokens[i].Project)
		tokens[i].Name = s.pii.OpenName("service_tokens.name", tokens[i].Name)
	}
	for i := range secrets {
		secrets[i].Project = s.pii.OpenName("projects.name", secrets[i].Project)
		secrets[i].Name = s.pii.OpenName("secrets.key", secrets[i].Name)
	}
	for _, t := range tokens {
		out.Attention = append(out.Attention, Attention{Kind: "token", Token: t.Name, ProjectID: t.ProjectID, Project: t.Project, Reason: tokenReason(t)})
	}
	for _, sec := range secrets {
		out.Attention = append(out.Attention, Attention{Kind: "secret", Token: sec.Name, ProjectID: sec.ProjectID, Project: sec.Project, Reason: secretReason(sec)})
	}
	return out, nil
}

func tokenReason(t ExpiryRow) string {
	now := time.Now()
	switch {
	case t.ExpiresAt != nil && t.ExpiresAt.Before(now):
		return "Expired"
	case t.ExpiresAt != nil && t.ExpiresAt.Before(now.Add(30*24*time.Hour)):
		return "Expires in " + days(*t.ExpiresAt) + " days"
	}
	return "Not used in 30+ days"
}

func secretReason(s ExpiryRow) string {
	if s.ExpiresAt.After(time.Now()) {
		return "Secret expires in " + days(*s.ExpiresAt) + " days"
	}
	return "Secret expired"
}

func days(t time.Time) string { return strconv.Itoa(int(time.Until(t).Hours()/24) + 1) }
