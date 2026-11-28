package token

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/audit"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
	"vaultly/internal/tokenutil"
)

// fakeTokens stores records by hash, as the real repository does.
type fakeTokens struct {
	TokenRepository
	service  map[string]*ServiceRecord
	personal map[string]*PersonalRecord
}

func (f *fakeTokens) ServiceByHash(_ context.Context, h []byte) (*ServiceRecord, error) {
	if r, ok := f.service[string(h)]; ok {
		return r, nil
	}
	return nil, errors.New("no rows")
}

func (f *fakeTokens) PersonalByHash(_ context.Context, h []byte) (*PersonalRecord, error) {
	if r, ok := f.personal[string(h)]; ok {
		return r, nil
	}
	return nil, errors.New("no rows")
}

func (f *fakeTokens) TouchService(context.Context, uuid.UUID)  {}
func (f *fakeTokens) TouchPersonal(context.Context, uuid.UUID) {}

type fixture struct {
	svc    *Service
	repo   *fakeTokens
	box    *pii.Box
	mu     sync.Mutex
	events []audit.Event
}

func newFixture() *fixture {
	key := func() []byte { b := make([]byte, 32); copy(b, uuid.New().String()); return b }
	f := &fixture{repo: &fakeTokens{service: map[string]*ServiceRecord{}, personal: map[string]*PersonalRecord{}}, box: pii.New(key(), key())}
	au := audit.New(nil, nil, "")
	au.Subscribe(func(e audit.Event) { f.mu.Lock(); f.events = append(f.events, e); f.mu.Unlock() })
	f.svc = NewService(f.repo, nil, f.box, au)
	return f
}

func (f *fixture) addService(rec ServiceRecord) string {
	raw, hash := tokenutil.New(tokenutil.PrefixService)
	rec.ID = uuid.New()
	f.repo.service[string(hash)] = &rec
	return raw
}

func ago(d time.Duration) *time.Time { t := time.Now().Add(-d); return &t }
func in(d time.Duration) *time.Time  { t := time.Now().Add(d); return &t }

func TestAuthenticateService(t *testing.T) {
	f := newFixture()
	valid := f.addService(ServiceRecord{EnvSlug: "prod", PathPrefix: "/stripe", ExpiresAt: in(time.Hour)})
	noExpiry := f.addService(ServiceRecord{EnvSlug: "prod", PathPrefix: "/"})
	expired := f.addService(ServiceRecord{EnvSlug: "prod", PathPrefix: "/", ExpiresAt: ago(time.Second)})
	fenced := f.addService(ServiceRecord{EnvSlug: "prod", PathPrefix: "/", CIDRs: []string{"10.0.0.0/8"}})
	personal, _ := tokenutil.New(tokenutil.PrefixPersonal)

	cases := []struct {
		name, raw, ip string
		want          error
	}{
		{"valid token", valid, "1.2.3.4", nil},
		{"token without expiry", noExpiry, "1.2.3.4", nil},
		{"expired token", expired, "1.2.3.4", ErrInvalidToken},
		{"unknown token", tokenutil.PrefixService + "nope", "1.2.3.4", ErrInvalidToken},
		{"personal token on machine endpoint", personal, "1.2.3.4", ErrNeedService},
		{"IP inside allowlist", fenced, "10.9.8.7", nil},
		{"IP outside allowlist", fenced, "192.168.0.1", ErrIPNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := f.svc.AuthenticateService(context.Background(), c.raw, c.ip)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if c.want == nil && (a.Token == nil || a.IP != c.ip) {
				t.Fatalf("actor not populated: %+v", a)
			}
		})
	}
}

func TestAuthenticateServiceCarriesScope(t *testing.T) {
	f := newFixture()
	raw := f.addService(ServiceRecord{EnvSlug: "prod", PathPrefix: "/stripe", CanWrite: false})
	a, err := f.svc.AuthenticateService(context.Background(), raw, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if a.Token.EnvSlug != "prod" || a.Token.PathPrefix != "/stripe" || a.Token.CanWrite {
		t.Fatalf("scope lost: %+v", a.Token)
	}
}

func TestIPDenialIsAudited(t *testing.T) {
	f := newFixture()
	raw := f.addService(ServiceRecord{Name: "ci", PathPrefix: "/", CIDRs: []string{"10.0.0.0/8"}})
	if _, err := f.svc.AuthenticateService(context.Background(), raw, "8.8.8.8"); !errors.Is(err, ErrIPNotAllowed) {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) != 1 || f.events[0].Action != "token.denied" || f.events[0].Metadata["reason"] != "ip_not_allowed" {
		t.Fatalf("expected one token.denied event, got %+v", f.events)
	}
}

func TestAuthenticatePersonal(t *testing.T) {
	f := newFixture()
	add := func(exp *time.Time) string {
		raw, hash := tokenutil.New(tokenutil.PrefixPersonal)
		f.repo.personal[string(hash)] = &PersonalRecord{ID: uuid.New(), UserID: uuid.New(), EmailEnc: f.box.Seal("users.email", "a@b.c"), ExpiresAt: exp}
		return raw
	}
	valid, expired := add(nil), add(ago(time.Minute))

	a, err := f.svc.AuthenticatePersonal(context.Background(), valid)
	if err != nil || !a.ViaPAT || a.Email != "a@b.c" {
		t.Fatalf("valid personal token: %+v, %v", a, err)
	}
	if _, err := f.svc.AuthenticatePersonal(context.Background(), expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired personal token: want invalid, got %v", err)
	}
}

func principalViaPAT() principal.Actor { return principal.Actor{UserID: uuid.New(), ViaPAT: true} }

func TestRevokePersonalNeedsPassword(t *testing.T) {
	// A leaked personal token must not be able to revoke the owner's other tokens.
	f := newFixture()
	if err := f.svc.RevokePersonal(context.Background(), principalViaPAT(), uuid.New()); !errors.Is(err, ErrNeedPassword) {
		t.Fatalf("got %v", err)
	}
}
