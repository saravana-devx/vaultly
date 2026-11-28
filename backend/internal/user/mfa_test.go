package user

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/auth"
	"vaultly/internal/pii"
	"vaultly/internal/tokenutil"
)

// fakeMFA mirrors the SQL semantics of the real repository, including the compare-and-set on the step.
type fakeMFA struct {
	mu    sync.Mutex
	rec   MFARecord
	codes map[string]bool // hash -> used
}

func (f *fakeMFA) MFA(context.Context, uuid.UUID) (*MFARecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.rec
	r.RecoveryLeft = 0
	for _, used := range f.codes {
		if !used {
			r.RecoveryLeft++
		}
	}
	return &r, nil
}

func (f *fakeMFA) SetPendingTOTP(_ context.Context, _ uuid.UUID, sealed string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rec.PendingEnc = sealed
	return nil
}

func (f *fakeMFA) EnableTOTP(_ context.Context, _ uuid.UUID, step int64, hashes [][]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rec.PendingEnc == "" {
		return ErrMFANoSetup
	}
	now := time.Now()
	f.rec.SecretEnc, f.rec.PendingEnc, f.rec.LastStep, f.rec.EnabledAt = f.rec.PendingEnc, "", step, &now
	f.setCodes(hashes)
	return nil
}

func (f *fakeMFA) DisableTOTP(context.Context, uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rec, f.codes = MFARecord{}, nil
	return nil
}

func (f *fakeMFA) ReplaceRecoveryCodes(_ context.Context, _ uuid.UUID, hashes [][]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCodes(hashes)
	return nil
}

func (f *fakeMFA) setCodes(hashes [][]byte) {
	f.codes = map[string]bool{}
	for _, h := range hashes {
		f.codes[string(h)] = false
	}
}

func (f *fakeMFA) AcceptTOTPStep(_ context.Context, _ uuid.UUID, step int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rec.LastStep >= step {
		return false, nil
	}
	f.rec.LastStep = step
	return true, nil
}

func (f *fakeMFA) UseRecoveryCode(_ context.Context, _ uuid.UUID, h []byte) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	used, ok := f.codes[string(h)]
	if !ok || used {
		return false, nil
	}
	f.codes[string(h)] = true
	return true, nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newMFA(t *testing.T) (*mfaService, *fakeMFA, *clock) {
	t.Helper()
	k := make([]byte, 32)
	copy(k, "test-key-material-0123456789abcd")
	box := pii.New(k, k)
	repo := &fakeMFA{}
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	return &mfaService{repo: repo, seal: box.Seal, open: box.Open, now: c.now}, repo, c
}

// enrol runs setup + enable and returns the plaintext secret and recovery codes.
func enrol(t *testing.T, m *mfaService, c *clock) (string, []string) {
	t.Helper()
	setup, err := m.setup(context.Background(), uuid.New(), "a@b.c")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := auth.TOTPCode(setup.Secret, auth.TOTPStep(c.t))
	codes, err := m.enable(context.Background(), uuid.New(), code)
	if err != nil {
		t.Fatal(err)
	}
	return setup.Secret, codes
}

func TestMFASetupStoresSecretSealed(t *testing.T) {
	m, repo, _ := newMFA(t)
	setup, err := m.setup(context.Background(), uuid.New(), "a@b.c")
	if err != nil {
		t.Fatal(err)
	}
	if repo.rec.PendingEnc == "" || repo.rec.PendingEnc == setup.Secret || !pii.Sealed(repo.rec.PendingEnc) {
		t.Fatalf("pending secret must be stored sealed, got %q", repo.rec.PendingEnc)
	}
	if repo.rec.SecretEnc != "" {
		t.Fatal("2FA must not be on until a code confirms setup")
	}
}

func TestMFAEnableNeedsValidCode(t *testing.T) {
	m, repo, _ := newMFA(t)
	if _, err := m.enable(context.Background(), uuid.New(), "123456"); !errors.Is(err, ErrMFANoSetup) {
		t.Fatalf("enable before setup: got %v", err)
	}
	if _, err := m.setup(context.Background(), uuid.New(), "a@b.c"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.enable(context.Background(), uuid.New(), "000000"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("wrong code: got %v", err)
	}
	if repo.rec.SecretEnc != "" {
		t.Fatal("a wrong code must not turn 2FA on")
	}
}

func TestMFAEnableReturnsRecoveryCodes(t *testing.T) {
	m, repo, c := newMFA(t)
	_, codes := enrol(t, m, c)
	if len(codes) != recoveryCodeCount {
		t.Fatalf("want %d codes, got %d", recoveryCodeCount, len(codes))
	}
	for h := range repo.codes {
		for _, c := range codes {
			if h == c {
				t.Fatal("recovery codes must be stored hashed")
			}
		}
	}
	if _, err := m.setup(context.Background(), uuid.New(), "a@b.c"); !errors.Is(err, ErrMFAAlreadyOn) {
		t.Fatalf("setup while on: got %v", err)
	}
	st, _ := m.status(context.Background(), uuid.New())
	if !st.Enabled || st.RecoveryLeft != recoveryCodeCount {
		t.Fatalf("status: %+v", st)
	}
}

func TestMFATOTPCannotBeReplayed(t *testing.T) {
	m, _, c := newMFA(t)
	secret, _ := enrol(t, m, c)

	// The code used to enable is already spent.
	enableCode, _ := auth.TOTPCode(secret, auth.TOTPStep(c.t))
	if _, err := m.verify(context.Background(), uuid.New(), enableCode); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("enable code replayed: %v", err)
	}

	c.t = c.t.Add(30 * time.Second)
	next, _ := auth.TOTPCode(secret, auth.TOTPStep(c.t))
	if method, err := m.verify(context.Background(), uuid.New(), next); err != nil || method != "totp" {
		t.Fatalf("fresh code: %s, %v", method, err)
	}
	if _, err := m.verify(context.Background(), uuid.New(), next); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("same code twice: %v", err)
	}

	// An older code still inside the drift window is also refused once a newer step was used.
	c.t = c.t.Add(30 * time.Second)
	if _, err := m.verify(context.Background(), uuid.New(), next); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("older step accepted: %v", err)
	}
}

func TestMFAConcurrentSameCodeOnlyOnce(t *testing.T) {
	m, _, c := newMFA(t)
	secret, _ := enrol(t, m, c)
	c.t = c.t.Add(30 * time.Second)
	code, _ := auth.TOTPCode(secret, auth.TOTPStep(c.t))

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.verify(context.Background(), uuid.New(), code); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("code accepted %d times, want exactly 1", wins)
	}
}

func TestMFARecoveryCodesAreSingleUse(t *testing.T) {
	m, _, c := newMFA(t)
	_, codes := enrol(t, m, c)

	if method, err := m.verify(context.Background(), uuid.New(), "  "+codes[0]+" "); err != nil || method != "recovery_code" {
		t.Fatalf("recovery code: %s, %v", method, err)
	}
	if _, err := m.verify(context.Background(), uuid.New(), codes[0]); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("recovery code reused: %v", err)
	}
	if _, err := m.verify(context.Background(), uuid.New(), "zzzzz-zzzzz"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("unknown recovery code: %v", err)
	}
	st, _ := m.status(context.Background(), uuid.New())
	if st.RecoveryLeft != recoveryCodeCount-1 {
		t.Fatalf("want %d left, got %d", recoveryCodeCount-1, st.RecoveryLeft)
	}
}

func TestMFARegenerateInvalidatesOldCodes(t *testing.T) {
	m, repo, c := newMFA(t)
	_, old := enrol(t, m, c)
	fresh, err := m.regenerate(context.Background(), uuid.New(), old[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, used := repo.codes[string(tokenutil.Hash(auth.NormRecoveryCode(old[1])))]; used {
		t.Fatal("old codes must be gone")
	}
	if _, err := m.verify(context.Background(), uuid.New(), old[1]); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("old code still works: %v", err)
	}
	if _, err := m.verify(context.Background(), uuid.New(), fresh[0]); err != nil {
		t.Fatalf("new code: %v", err)
	}
}

func TestMFAVerifyWhenOff(t *testing.T) {
	m, _, _ := newMFA(t)
	if _, err := m.verify(context.Background(), uuid.New(), "123456"); !errors.Is(err, ErrMFANotEnabled) {
		t.Fatalf("got %v", err)
	}
}
