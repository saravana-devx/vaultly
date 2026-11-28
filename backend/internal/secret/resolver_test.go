package secret

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"vaultly/internal/apperr"
	"vaultly/internal/crypto"
	"vaultly/internal/principal"
)

// store is an in-memory LiveReader + EnvLookup holding ciphertext, like the real repository.
type store struct {
	t       *testing.T
	project uuid.UUID
	dek     []byte
	envs    map[string]uuid.UUID
	data    map[string][]byte // envID|path|key -> ciphertext
	reads   int
}

func newStore(t *testing.T) *store {
	dek := make([]byte, 32)
	rand.Read(dek)
	return &store{t: t, project: uuid.New(), dek: dek, envs: map[string]uuid.UUID{}, data: map[string][]byte{}}
}

func (s *store) put(env, path, key, val string) {
	id, ok := s.envs[env]
	if !ok {
		id = uuid.New()
		s.envs[env] = id
	}
	ct, err := crypto.Encrypt(s.dek, []byte(val), []byte(s.project.String()))
	if err != nil {
		s.t.Fatal(err)
	}
	s.data[id.String()+"|"+path+"|"+key] = ct
}

func (s *store) EnvID(_ context.Context, _ uuid.UUID, slug string) (uuid.UUID, error) {
	if id, ok := s.envs[slug]; ok {
		return id, nil
	}
	return uuid.Nil, errors.New("no env")
}

func (s *store) Lookup(_ context.Context, envID uuid.UUID, path, key string) ([]byte, error) {
	s.reads++
	if ct, ok := s.data[envID.String()+"|"+path+"|"+key]; ok {
		return ct, nil
	}
	return nil, errors.New("not found")
}

func (s *store) resolve(val, env, path string, allow func(env, path string) bool) string {
	r := newResolver(context.Background(), s.project, s.dek, allow, s, s)
	return r.expand(val, env, path, map[string]bool{}, 0)
}

func TestResolveForms(t *testing.T) {
	s := newStore(t)
	s.put("dev", "/", "HOST", "db.local")
	s.put("dev", "/", "PORT", "5432")
	s.put("dev", "/stripe", "KEY", "sk_dev")
	s.put("prod", "/", "HOST", "db.prod")
	s.put("prod", "/stripe", "KEY", "sk_live")

	cases := []struct {
		name, val, path, want string
	}{
		{"no reference is untouched", "plain", "/", "plain"},
		{"same folder", "${HOST}:${PORT}", "/", "db.local:5432"},
		{"relative falls back to root", "${HOST}", "/stripe", "db.local"},
		{"relative prefers own folder", "${KEY}", "/stripe", "sk_dev"},
		{"absolute path", "${/stripe/KEY}", "/", "sk_dev"},
		{"absolute root key", "${/HOST}", "/stripe", "db.local"},
		{"other environment", "${prod:/HOST}", "/", "db.prod"},
		{"other environment nested", "${prod:/stripe/KEY}", "/", "sk_live"},
		{"missing key left as-is", "${NOPE}", "/", "${NOPE}"},
		{"unknown environment left as-is", "${qa:/HOST}", "/", "${qa:/HOST}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.resolve(c.val, "dev", c.path, nil); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestResolveChained(t *testing.T) {
	s := newStore(t)
	s.put("dev", "/", "USER", "app")
	s.put("dev", "/", "HOST", "db.local")
	s.put("dev", "/", "DSN", "postgres://${USER}@${HOST}")
	if got := s.resolve("${DSN}/main", "dev", "/", nil); got != "postgres://app@db.local/main" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCycleTerminates(t *testing.T) {
	s := newStore(t)
	s.put("dev", "/", "A", "a-${B}")
	s.put("dev", "/", "B", "b-${A}")
	s.put("dev", "/", "SELF", "x${SELF}")

	// The cycle is cut where it closes: the repeated reference stays literal.
	if got := s.resolve("${A}", "dev", "/", nil); got != "a-b-${A}" {
		t.Fatalf("got %q", got)
	}
	if got := s.resolve("${SELF}", "dev", "/", nil); got != "x${SELF}" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDepthLimit(t *testing.T) {
	s := newStore(t)
	// K0 -> K1 -> ... -> K20: a long but acyclic chain must stop at the depth limit, not recurse forever.
	for i := 0; i < 20; i++ {
		s.put("dev", "/", key(i), "${"+key(i+1)+"}")
	}
	s.put("dev", "/", key(20), "end")
	got := s.resolve("${K0}", "dev", "/", nil)
	if got == "end" {
		t.Fatal("chain longer than the depth limit was fully resolved")
	}
	if s.reads > 20 {
		t.Fatalf("too many reads: %d", s.reads)
	}
}

func key(i int) string { return fmt.Sprintf("K%d", i) }

func TestResolveRespectsScope(t *testing.T) {
	s := newStore(t)
	s.put("dev", "/", "PUBLIC", "ok")
	s.put("dev", "/stripe", "KEY", "sk_dev")
	s.put("dev", "/app", "LEAK", "${/stripe/KEY}")
	s.put("prod", "/", "HOST", "db.prod")

	// A token scoped to dev:/app may read /app and nothing else.
	allow := func(env, p string) bool { return env == "dev" && (p == "/app") }

	cases := []struct{ name, val, want string }{
		{"cannot reach sibling folder", "${/stripe/KEY}", "${/stripe/KEY}"},
		{"cannot reach another environment", "${prod:/HOST}", "${prod:/HOST}"},
		{"root fallback is also scoped", "${PUBLIC}", "${PUBLIC}"},
		{"indirection does not escape scope", "${LEAK}", "${/stripe/KEY}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.resolve(c.val, "dev", "/app", allow); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestResolveWrongKeyFailsClosed(t *testing.T) {
	s := newStore(t)
	s.put("dev", "/", "HOST", "db.local")
	other := make([]byte, 32)
	rand.Read(other)
	r := newResolver(context.Background(), s.project, other, nil, s, s)
	if got := r.expand("${HOST}", "dev", "/", map[string]bool{}, 0); got != "${HOST}" {
		t.Fatalf("decrypted with the wrong key: %q", got)
	}
}

func TestResolveProjectBoundAAD(t *testing.T) {
	// Ciphertext is bound to its project ID: the same DEK under another project ID must not open it.
	s := newStore(t)
	s.put("dev", "/", "HOST", "db.local")
	r := newResolver(context.Background(), uuid.New(), s.dek, nil, s, s)
	if got := r.expand("${HOST}", "dev", "/", map[string]bool{}, 0); got != "${HOST}" {
		t.Fatalf("ciphertext opened under a different project: %q", got)
	}
}

func TestTokenPath(t *testing.T) {
	tok := &principal.ServiceToken{PathPrefix: "/stripe"}
	cases := []struct {
		req, want string
		ok        bool
	}{
		{"", "/stripe", true},
		{"/stripe", "/stripe", true},
		{"stripe/live/", "/stripe/live", true},
		{"/", "", false},
		{"/stripe2", "", false},
		{"/stripe/../other", "", false},
		{"/other", "", false},
	}
	for _, c := range cases {
		got, err := tokenPath(tok, c.req)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("tokenPath(%q) = %q, %v; want %q", c.req, got, err, c.want)
			}
			continue
		}
		if ae, ok := apperr.As(err); !ok || ae.Kind != apperr.KindForbidden {
			t.Errorf("tokenPath(%q): want forbidden, got %q, %v", c.req, got, err)
		}
	}
}
