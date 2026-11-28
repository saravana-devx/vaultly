package user

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/auth"
	"vaultly/internal/principal"
	"vaultly/internal/tokenutil"
)

const (
	fTOTP             = "users.totp_secret"
	recoveryCodeCount = 10
)

// MFARepository is the storage 2FA needs; *Repository implements it.
type MFARepository interface {
	MFA(ctx context.Context, userID uuid.UUID) (*MFARecord, error)
	SetPendingTOTP(ctx context.Context, userID uuid.UUID, sealed string) error
	EnableTOTP(ctx context.Context, userID uuid.UUID, step int64, codeHashes [][]byte) error
	DisableTOTP(ctx context.Context, userID uuid.UUID) error
	ReplaceRecoveryCodes(ctx context.Context, userID uuid.UUID, codeHashes [][]byte) error
	AcceptTOTPStep(ctx context.Context, userID uuid.UUID, step int64) (bool, error)
	UseRecoveryCode(ctx context.Context, userID uuid.UUID, hash []byte) (bool, error)
}

type MFAStatus struct {
	Enabled      bool       `json:"enabled"`
	EnabledAt    *time.Time `json:"enabled_at"`
	RecoveryLeft int        `json:"recovery_codes_left"`
	SetupStarted bool       `json:"setup_started"`
}

type MFASetup struct {
	Secret string `json:"secret"`
	URL    string `json:"otpauth_url"`
	QR     string `json:"qr_png"`
}

// mfaService holds the 2FA logic. It is separate from Service so it can be tested without Redis.
type mfaService struct {
	repo MFARepository
	seal func(field, plain string) string
	open func(field, sealed string) (string, error)
	now  func() time.Time
}

// verify checks a second factor: a 6-digit TOTP code, or a recovery code (anything longer).
// A TOTP code is accepted only if its time step is newer than the last one used, so a code
// seen over someone's shoulder can't be replayed inside its 30-second window.
func (m *mfaService) verify(ctx context.Context, userID uuid.UUID, code string) (method string, err error) {
	rec, err := m.repo.MFA(ctx, userID)
	if err != nil {
		return "", err
	}
	if !rec.Enabled() {
		return "", ErrMFANotEnabled
	}
	code = strings.TrimSpace(code)
	if len(strings.ReplaceAll(code, " ", "")) > 6 {
		ok, err := m.repo.UseRecoveryCode(ctx, userID, tokenutil.Hash(auth.NormRecoveryCode(code)))
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrInvalidMFACode
		}
		return "recovery_code", nil
	}
	secret, err := m.open(fTOTP, rec.SecretEnc)
	if err != nil {
		return "", err
	}
	step, ok := auth.VerifyTOTP(secret, code, m.now())
	if !ok || step <= rec.LastStep {
		return "", ErrInvalidMFACode
	}
	if ok, err := m.repo.AcceptTOTPStep(ctx, userID, step); err != nil || !ok {
		return "", ErrInvalidMFACode // lost a race with another request using the same code
	}
	return "totp", nil
}

func (m *mfaService) status(ctx context.Context, userID uuid.UUID) (*MFAStatus, error) {
	rec, err := m.repo.MFA(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MFAStatus{Enabled: rec.Enabled(), EnabledAt: rec.EnabledAt, RecoveryLeft: rec.RecoveryLeft, SetupStarted: rec.PendingEnc != ""}, nil
}

func (m *mfaService) setup(ctx context.Context, userID uuid.UUID, account string) (*MFASetup, error) {
	rec, err := m.repo.MFA(ctx, userID)
	if err != nil {
		return nil, err
	}
	if rec.Enabled() {
		return nil, ErrMFAAlreadyOn
	}
	secret := auth.NewTOTPSecret()
	if err := m.repo.SetPendingTOTP(ctx, userID, m.seal(fTOTP, secret)); err != nil {
		return nil, err
	}
	u := auth.TOTPURL(secret, account)
	qr, err := auth.TOTPQR(u)
	if err != nil {
		return nil, err
	}
	return &MFASetup{Secret: secret, URL: u, QR: qr}, nil
}

// enable confirms setup with a code from the app, then returns fresh recovery codes (shown once).
func (m *mfaService) enable(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	rec, err := m.repo.MFA(ctx, userID)
	if err != nil {
		return nil, err
	}
	if rec.Enabled() {
		return nil, ErrMFAAlreadyOn
	}
	if rec.PendingEnc == "" {
		return nil, ErrMFANoSetup
	}
	secret, err := m.open(fTOTP, rec.PendingEnc)
	if err != nil {
		return nil, err
	}
	step, ok := auth.VerifyTOTP(secret, code, m.now())
	if !ok {
		return nil, ErrInvalidMFACode
	}
	codes, hashes := newRecoveryCodes()
	if err := m.repo.EnableTOTP(ctx, userID, step, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

func (m *mfaService) regenerate(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	if _, err := m.verify(ctx, userID, code); err != nil {
		return nil, err
	}
	codes, hashes := newRecoveryCodes()
	return codes, m.repo.ReplaceRecoveryCodes(ctx, userID, hashes)
}

func newRecoveryCodes() ([]string, [][]byte) {
	codes := auth.NewRecoveryCodes(recoveryCodeCount)
	hashes := make([][]byte, len(codes))
	for i, c := range codes {
		hashes[i] = tokenutil.Hash(auth.NormRecoveryCode(c))
	}
	return codes, hashes
}

// --- Service-level entry points (sessions, audit, password checks) ---

func (s *Service) LoginMFA(ctx context.Context, req *LoginMFARequest, ip string) (*Session, error) {
	userID, err := s.sessions.PeekMFAChallenge(ctx, req.Challenge)
	if err != nil {
		return nil, ErrMFAChallengeExpired
	}
	method, err := s.mfa.verify(ctx, userID, req.Code)
	if err != nil {
		if err == ErrInvalidMFACode {
			s.sessions.FailMFAChallenge(ctx, req.Challenge)
		}
		return nil, err
	}
	if !s.sessions.ConsumeMFAChallenge(ctx, req.Challenge) {
		return nil, ErrMFAChallengeExpired // redeemed concurrently
	}
	profile, err := s.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.recordUser(ctx, principal.Actor{UserID: profile.ID, Email: profile.Email, IP: ip}, "auth.login", map[string]any{"mfa": method})
	return s.issue(ctx, profile)
}

func (s *Service) MFAStatus(ctx context.Context, a principal.Actor) (*MFAStatus, error) {
	return s.mfa.status(ctx, a.UserID)
}

func (s *Service) MFASetup(ctx context.Context, a principal.Actor) (*MFASetup, error) {
	if a.ViaPAT {
		return nil, ErrMFANeedSession
	}
	return s.mfa.setup(ctx, a.UserID, a.Email)
}

func (s *Service) MFAEnable(ctx context.Context, a principal.Actor, code string) ([]string, error) {
	if a.ViaPAT {
		return nil, ErrMFANeedSession
	}
	codes, err := s.mfa.enable(ctx, a.UserID, code)
	if err == nil {
		s.recordUser(ctx, a, "auth.mfa_enabled", nil)
	}
	return codes, err
}

// MFADisable needs the password AND a current code, so a stolen session alone can't turn 2FA off.
func (s *Service) MFADisable(ctx context.Context, a principal.Actor, req *MFADisableRequest) error {
	if a.ViaPAT {
		return ErrMFANeedSession
	}
	rec, err := s.repo.ByID(ctx, a.UserID)
	if err != nil {
		return err
	}
	if !auth.CheckPassword(rec.PasswordHash, req.Password) {
		return ErrWrongPassword
	}
	if _, err := s.mfa.verify(ctx, a.UserID, req.Code); err != nil {
		return err
	}
	if err := s.mfa.repo.DisableTOTP(ctx, a.UserID); err != nil {
		return err
	}
	s.recordUser(ctx, a, "auth.mfa_disabled", nil)
	return nil
}

func (s *Service) MFARegenerate(ctx context.Context, a principal.Actor, code string) ([]string, error) {
	if a.ViaPAT {
		return nil, ErrMFANeedSession
	}
	codes, err := s.mfa.regenerate(ctx, a.UserID, code)
	if err == nil {
		s.recordUser(ctx, a, "auth.mfa_recovery_regenerated", nil)
	}
	return codes, err
}

// recordUser audits an account-level event against the user's first workspace.
func (s *Service) recordUser(ctx context.Context, a principal.Actor, action string, md map[string]any) {
	if orgID, ok, _ := s.repo.FirstOrg(ctx, a.UserID); ok {
		s.audit.Record(ctx, a, orgID, nil, action, "", md)
	}
}
