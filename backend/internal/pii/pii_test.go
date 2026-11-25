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
	s1, s2 := b.Seal("users.email", "maya@acme.io"), b.Seal("users.email", "maya@acme.io")
	if !Sealed(s1) || s1 == s2 || strings.Contains(s1, "maya") {
		t.Fatalf("sealed values must be opaque and randomised: %q %q", s1, s2)
	}
	got, err := b.Open("users.email", s1)
	if err != nil || got != "maya@acme.io" {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestFieldNameIsBoundToTheCiphertext(t *testing.T) {
	b := box()
	s := b.Seal("users.email", "maya@acme.io")
	if _, err := b.Open("users.name", s); err == nil {
		t.Fatal("a value sealed for one column must not open as another column")
	}
	if _, err := box().Open("users.email", s); err == nil {
		t.Fatal("a different key must not open it")
	}
}

func TestBlindIndexIsStableCaseInsensitiveAndKeyed(t *testing.T) {
	b := box()
	if !bytes.Equal(b.Index("Maya@Acme.io "), b.Index("maya@acme.io")) {
		t.Fatal("index must ignore case and surrounding spaces")
	}
	if bytes.Equal(b.Index("maya@acme.io"), b.Index("dev@acme.io")) {
		t.Fatal("different emails must differ")
	}
	if bytes.Equal(b.Index("maya@acme.io"), box().Index("maya@acme.io")) {
		t.Fatal("the index must depend on the secret key (otherwise it is a plain hash that can be brute-forced)")
	}
	if len(b.Index("x@y.z")) != 32 {
		t.Fatal("expected a SHA-256 sized index")
	}
}

func TestLabelSealsEmailsOnlyAndIsIdempotent(t *testing.T) {
	b := box()
	l := b.Label("maya@acme.io")
	if !Sealed(l) || b.Label(l) != l {
		t.Fatalf("emails must be sealed exactly once: %q", l)
	}
	for _, keep := range []string{"token:github-actions", "system", "", "/stripe/KEY"} {
		if b.Label(keep) != keep {
			t.Errorf("%q must pass through unchanged", keep)
		}
	}
	if b.OpenLabel(l) != "maya@acme.io" || b.OpenLabel("legacy@plain.io") != "legacy@plain.io" {
		t.Fatal("OpenLabel must decrypt sealed values and pass plaintext through")
	}
	if b.OpenLabel(box().Label("x@y.z")) != "[unreadable]" {
		t.Fatal("a value sealed under another key must degrade to a placeholder, not an error")
	}
}
