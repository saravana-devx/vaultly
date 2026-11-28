package pii

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func box() *Box {
	a, b := make([]byte, 32), make([]byte, 32)
	rand.Read(a)
	rand.Read(b)
	return New(a, b)
}

func TestSealOpenRoundTripAndRandomised(t *testing.T) {
	b := box()
	s1, s2 := b.Seal("users.email", "lily@vaultly.dev"), b.Seal("users.email", "lily@vaultly.dev")
	if !Sealed(s1) || s1 == s2 || strings.Contains(s1, "lily") {
		t.Fatalf("sealed values must be opaque and randomised: %q %q", s1, s2)
	}
	got, err := b.Open("users.email", s1)
	if err != nil || got != "lily@vaultly.dev" {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestFieldNameIsBoundToTheCiphertext(t *testing.T) {
	b := box()
	s := b.Seal("users.email", "lily@vaultly.dev")
	if _, err := b.Open("users.name", s); err == nil {
		t.Fatal("a value sealed for one column must not open as another column")
	}
	if _, err := box().Open("users.email", s); err == nil {
		t.Fatal("a different key must not open it")
	}
}

func TestBlindIndexIsStableCaseInsensitiveAndKeyed(t *testing.T) {
	b := box()
	if !bytes.Equal(b.Index("Lily@Vaultly.dev "), b.Index("lily@vaultly.dev")) {
		t.Fatal("index must ignore case and surrounding spaces")
	}
	if bytes.Equal(b.Index("lily@vaultly.dev"), b.Index("rose@vaultly.dev")) {
		t.Fatal("different emails must differ")
	}
	if bytes.Equal(b.Index("lily@vaultly.dev"), box().Index("lily@vaultly.dev")) {
		t.Fatal("the index must depend on the secret key (otherwise it is a plain hash that can be brute-forced)")
	}
	if len(b.Index("x@y.z")) != 32 {
		t.Fatal("expected a SHA-256 sized index")
	}
}

func TestLabelSealsEmailsOnlyAndIsIdempotent(t *testing.T) {
	b := box()
	l := b.Label("lily@vaultly.dev")
	if !Sealed(l) || b.Label(l) != l {
		t.Fatalf("emails must be sealed exactly once: %q", l)
	}
	for _, keep := range []string{"token:github-actions", "system", "", "/stripe/KEY"} {
		if b.Label(keep) != keep {
			t.Errorf("%q must pass through unchanged", keep)
		}
	}
	if b.OpenLabel(l) != "lily@vaultly.dev" || b.OpenLabel("legacy@plain.io") != "legacy@plain.io" {
		t.Fatal("OpenLabel must decrypt sealed values and pass plaintext through")
	}
	if b.OpenLabel(box().Label("x@y.z")) != "[unreadable]" {
		t.Fatal("a value sealed under another key must degrade to a placeholder, not an error")
	}
}

func TestNamesAreSealedIdempotentAndLegacyPlaintextPasses(t *testing.T) {
	b := box()
	s := b.SealName("orgs.name", "Rose Bennett's workspace")
	if !Sealed(s) || strings.Contains(s, "Rose") {
		t.Fatalf("name must be opaque: %q", s)
	}
	if b.SealName("orgs.name", s) != s {
		t.Fatal("sealing an already sealed value must not double-encrypt")
	}
	if got := b.OpenName("orgs.name", s); got != "Rose Bennett's workspace" {
		t.Fatalf("open: %q", got)
	}
	if got := b.OpenName("orgs.name", "legacy plaintext"); got != "legacy plaintext" {
		t.Fatalf("legacy plaintext must pass through: %q", got)
	}
	if got := b.OpenName("secrets.key", s); got != "[unreadable]" {
		t.Fatalf("a value bound to another field must not open: %q", got)
	}
	if b.SealName("orgs.name", "") != "" {
		t.Fatal("empty stays empty")
	}
}

func TestPolicyDecidesWhatIsSealed(t *testing.T) {
	b := box()
	for _, f := range []string{"orgs.name", "secrets.key", "secrets.comment", "change_requests.comment", "change_requests.note", "webhooks.url", "service_tokens.cidrs"} {
		if !Encrypted(f) || !Sealed(b.SealName(f, "x")) {
			t.Errorf("%s must be sealed", f)
		}
	}
	for _, f := range []string{"projects.name", "environments.slug", "secrets.path", "secrets.tags", "webhooks.name", "audit.metadata"} {
		if Encrypted(f) || b.SealName(f, "x") != "x" {
			t.Errorf("%s must stay readable", f)
		}
	}
}

func TestHashIsKeyedCaseSensitiveAndPerField(t *testing.T) {
	b := box()
	if !bytes.Equal(b.Hash("secrets.key", "DB_URL"), b.Hash("secrets.key", "DB_URL")) {
		t.Fatal("same input must hash the same")
	}
	if bytes.Equal(b.Hash("secrets.key", "DB_URL"), b.Hash("secrets.key", "db_url")) {
		t.Fatal("keys are case sensitive")
	}
	if bytes.Equal(b.Hash("secrets.key", "x"), b.Hash("environments.slug", "x")) {
		t.Fatal("different fields must not share hashes")
	}
	if bytes.Equal(box().Hash("secrets.key", "x"), b.Hash("secrets.key", "x")) {
		t.Fatal("hash must depend on the key")
	}
}

func TestJSONRoundTripAndLegacyJSON(t *testing.T) {
	b := box()
	sealed := b.SealJSON("service_tokens.cidrs", []string{"10.0.0.0/8", "192.168.1.5/32"})
	if !Sealed(sealed) || strings.Contains(sealed, "10.0.0.0") {
		t.Fatalf("must be opaque: %q", sealed)
	}
	var got []string
	if err := b.OpenJSON("service_tokens.cidrs", sealed, &got); err != nil || len(got) != 2 || got[0] != "10.0.0.0/8" {
		t.Fatalf("round trip: %v %v", got, err)
	}
	var legacy []string
	if err := b.OpenJSON("secrets.tags", `["old"]`, &legacy); err != nil || len(legacy) != 1 {
		t.Fatalf("legacy plaintext JSON must read: %v %v", legacy, err)
	}
}
