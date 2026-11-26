package user

import (
	"context"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"vaultly/internal/audit"
	"vaultly/internal/auth"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
)

type UserRepository interface {
	Create(ctx context.Context, u NewUser) (userID, orgID uuid.UUID, err error)
	ByEmailHash(ctx context.Context, hash []byte) (*Record, error)
	ByID(ctx context.Context, id uuid.UUID) (*Record, error)
	FirstOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, bool, error)
}

type Service struct {
	repo      UserRepository
	pii       *pii.Box
	sessions  *auth.Manager
	audit     *audit.Service
	dummyHash string
}

func NewService(repo UserRepository, box *pii.Box, sessions *auth.Manager, audit *audit.Service) *Service {
	dummy, _ := auth.HashPassword("dummy-password-for-timing")
	return &Service{repo: repo, pii: box, sessions: sessions, audit: audit, dummyHash: dummy}
}

func NormEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s *Service) Register(ctx context.Context, req *RegisterRequest, ip string) (*Session, error) {
	email, name := NormEmail(req.Email), strings.TrimSpace(req.Name)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidEmail
	}
	if name == "" {
		return nil, ErrNameRequired
	}
	orgName := strings.TrimSpace(req.OrgName)
	if orgName == "" {
		orgName = name + "'s workspace"
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	id, orgID, err := s.repo.Create(ctx, NewUser{
		EmailEnc: s.pii.Seal("users.email", email), EmailHash: s.pii.Index(email),
		NameEnc: s.pii.Seal("users.name", name), PasswordHash: hash, OrgName: s.pii.SealName("orgs.name", orgName),
	})
	if err != nil {
		return nil, err
	}
	profile := Profile{ID: id, Email: email, Name: name}
	s.audit.Record(ctx, principal.Actor{UserID: id, Email: email, IP: ip}, orgID, nil, "auth.register", "", nil)
	return s.issue(ctx, profile)
}

func (s *Service) Login(ctx context.Context, req *LoginRequest, ip string) (*Session, error) {
	rec, err := s.repo.ByEmailHash(ctx, s.pii.Index(req.Email))
	if err != nil {
		auth.CheckPassword(s.dummyHash, req.Password) // keep timing equal for unknown emails
		return nil, ErrInvalidCredentials
	}
	if !auth.CheckPassword(rec.PasswordHash, req.Password) {
		return nil, ErrInvalidCredentials
	}
	profile, err := s.open(rec)
	if err != nil {
		return nil, err
	}
	if orgID, ok, _ := s.repo.FirstOrg(ctx, profile.ID); ok {
		s.audit.Record(ctx, principal.Actor{UserID: profile.ID, Email: profile.Email, IP: ip}, orgID, nil, "auth.login", "", nil)
	}
	return s.issue(ctx, profile)
}

// Refresh rotates the refresh token: the presented one is consumed and a new pair issued.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Session, error) {
	if refreshToken == "" {
		return nil, ErrNoSession
	}
	id, err := s.sessions.ConsumeRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrSessionExpired
	}
	profile, err := s.Profile(ctx, id)
	if err != nil {
		return nil, ErrSessionExpired
	}
	return s.issue(ctx, profile)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) {
	if refreshToken != "" {
		s.sessions.RevokeRefreshToken(ctx, refreshToken)
	}
}

func (s *Service) Profile(ctx context.Context, id uuid.UUID) (Profile, error) {
	rec, err := s.repo.ByID(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	return s.open(rec)
}

// IDByEmail finds a user through the blind index; it never reads the encrypted email.
func (s *Service) IDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	rec, err := s.repo.ByEmailHash(ctx, s.pii.Index(email))
	if err != nil {
		return uuid.Nil, err
	}
	return rec.ID, nil
}

func (s *Service) EmailExists(ctx context.Context, email string) bool {
	_, err := s.IDByEmail(ctx, email)
	return err == nil
}

// OpenFields decrypts an email and name read straight from another table.
func (s *Service) OpenFields(emailEnc, nameEnc string) (email, name string, err error) {
	if email, err = s.pii.Open("users.email", emailEnc); err != nil {
		return "", "", err
	}
	name, err = s.pii.Open("users.name", nameEnc)
	return email, name, err
}

func (s *Service) open(rec *Record) (Profile, error) {
	email, err := s.pii.Open("users.email", rec.EmailEnc)
	if err != nil {
		return Profile{}, err
	}
	name, err := s.pii.Open("users.name", rec.NameEnc)
	if err != nil {
		return Profile{}, err
	}
	return Profile{ID: rec.ID, Email: email, Name: name}, nil
}

func (s *Service) issue(ctx context.Context, p Profile) (*Session, error) {
	access, err := s.sessions.NewAccessToken(p.ID, p.Email)
	if err != nil {
		return nil, err
	}
	refresh, err := s.sessions.NewRefreshToken(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &Session{AccessToken: access, RefreshToken: refresh, User: p}, nil
}
