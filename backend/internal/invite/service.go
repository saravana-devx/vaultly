package invite

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/apperr"
	"vaultly/internal/audit"
	mailer "vaultly/internal/mail"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
	"vaultly/internal/slug"
	"vaultly/internal/tokenutil"
	"vaultly/internal/user"
)

const ttl = 7 * 24 * time.Hour

type InviteRepository interface {
	IsMember(ctx context.Context, org uuid.UUID, emailHash []byte) (bool, error)
	ReplacePending(ctx context.Context, n NewInvite) (uuid.UUID, error)
	OrgName(ctx context.Context, org uuid.UUID) (string, error)
	ListPending(ctx context.Context, org uuid.UUID) ([]Invite, error)
	Delete(ctx context.Context, id, org uuid.UUID) (string, error)
	ByTokenHash(ctx context.Context, hash []byte) (*Invite, error)
	Accept(ctx context.Context, i *Invite, userID uuid.UUID) error
}

type Service struct {
	repo   InviteRepository
	access *access.Service
	users  *user.Service
	pii    *pii.Box
	mail   mailer.Sender
	audit  *audit.Service
	appURL string
}

func NewService(repo InviteRepository, access *access.Service, users *user.Service, box *pii.Box, mail mailer.Sender, audit *audit.Service, appURL string) *Service {
	return &Service{repo: repo, access: access, users: users, pii: box, mail: mail, audit: audit, appURL: appURL}
}

func (s *Service) Create(ctx context.Context, a principal.Actor, org uuid.UUID, req *CreateInviteRequest) (*Created, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	email := user.NormEmail(req.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidEmail
	}
	envRoles := map[string]string{}
	for name, role := range req.EnvRoles {
		if role == "" || role == "inherit" {
			continue
		}
		if slug.Make(name) != name || !access.ValidEnvRole(role) {
			return nil, ErrInvalidEnvRole
		}
		envRoles[name] = role
	}
	hash := s.pii.Index(email)
	if member, err := s.repo.IsMember(ctx, org, hash); err != nil {
		return nil, err
	} else if member {
		return nil, ErrAlreadyMember
	}

	raw, tokenHash := tokenutil.New(tokenutil.PrefixInvite)
	id, err := s.repo.ReplacePending(ctx, NewInvite{
		OrgID: org, EmailEnc: s.pii.Seal("invites.email", email), EmailHash: hash, Role: req.Role, EnvRoles: envRoles,
		TokenHash: tokenHash, InvitedBy: s.pii.Label(a.Label()), ExpiresAt: time.Now().Add(ttl),
	})
	if err != nil {
		return nil, err
	}
	link := s.appURL + "/invite/" + raw
	orgName, _ := s.repo.OrgName(ctx, org)
	go s.sendEmail(email, orgName, a.Label(), req.Role, link)
	s.audit.Record(ctx, a, org, nil, "member.invite", email, map[string]any{"role": req.Role})
	return &Created{ID: id, Email: email, Role: req.Role, Link: link}, nil
}

func (s *Service) sendEmail(to, orgName, inviter, role, link string) {
	text := fmt.Sprintf("%s invited you to join %s on Vaultly as %s.\n\nAccept the invitation: %s\n\nThis link expires in 7 days.", inviter, orgName, role, link)
	body := fmt.Sprintf(`<p><b>%s</b> invited you to join <b>%s</b> on Vaultly as <b>%s</b>.</p><p><a href="%s">Accept the invitation</a></p><p>This link expires in 7 days.</p>`,
		html.EscapeString(inviter), html.EscapeString(orgName), html.EscapeString(role), html.EscapeString(link))
	if err := s.mail.Send(to, "You're invited to "+orgName+" on Vaultly", text, body); err != nil {
		log.Printf("invite email failed: %v", err)
	}
}

func (s *Service) List(ctx context.Context, a principal.Actor, org uuid.UUID) ([]View, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListPending(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		email, err := s.pii.Open("invites.email", r.EmailEnc)
		if err != nil {
			continue
		}
		out = append(out, View{ID: r.ID, Email: email, Role: r.Role, EnvRoles: r.EnvRoles, InvitedBy: s.pii.OpenLabel(r.InvitedBy),
			ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt, Expired: r.ExpiresAt.Before(time.Now())})
	}
	return out, nil
}

func (s *Service) Revoke(ctx context.Context, a principal.Actor, org, id uuid.UUID) error {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return err
	}
	emailEnc, err := s.repo.Delete(ctx, id, org)
	if err != nil {
		return err
	}
	email, _ := s.pii.Open("invites.email", emailEnc)
	s.audit.Record(ctx, a, org, nil, "member.invite_revoke", email, nil)
	return nil
}

func (s *Service) Preview(ctx context.Context, token string) (*Preview, error) {
	inv, email, err := s.load(ctx, token)
	if err != nil {
		return nil, err
	}
	state := "pending"
	switch {
	case inv.Accepted != nil:
		state = "accepted"
	case inv.ExpiresAt.Before(time.Now()):
		state = "expired"
	}
	return &Preview{Org: inv.OrgName, EmailHint: maskEmail(email), Role: inv.Role, InvitedBy: s.pii.OpenLabel(inv.InvitedBy),
		State: state, HasAccount: s.users.EmailExists(ctx, email)}, nil
}

func (s *Service) Accept(ctx context.Context, a principal.Actor, token string) (*Accepted, error) {
	inv, email, err := s.load(ctx, token)
	if err != nil {
		return nil, err
	}
	if inv.Accepted != nil || inv.ExpiresAt.Before(time.Now()) {
		return nil, ErrInviteUsed
	}
	if !strings.EqualFold(a.Email, email) {
		return nil, apperr.Forbidden(fmt.Sprintf(MsgWrongAccountFmt, maskEmail(email)))
	}
	if err := s.repo.Accept(ctx, inv, a.UserID); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, inv.OrgID, nil, "member.join", a.Email, map[string]any{"role": inv.Role})
	return &Accepted{OrgID: inv.OrgID, Org: inv.OrgName, Role: inv.Role}, nil
}

func (s *Service) load(ctx context.Context, token string) (*Invite, string, error) {
	inv, err := s.repo.ByTokenHash(ctx, tokenutil.Hash(token))
	if err != nil {
		return nil, "", err
	}
	email, err := s.pii.Open("invites.email", inv.EmailEnc)
	if err != nil {
		return nil, "", ErrInviteNotFound
	}
	return inv, email, nil
}

func maskEmail(e string) string {
	at := strings.Index(e, "@")
	if at < 2 {
		return e
	}
	return e[:1] + strings.Repeat("•", at-1) + e[at:]
}
