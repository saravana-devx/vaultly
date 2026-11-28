package tokenutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	raw, hash := New(PrefixService)
	if !strings.HasPrefix(raw, PrefixService) {
		t.Fatalf("missing prefix: %q", raw)
	}
	// 32 random bytes in unpadded base64url is 43 characters.
	if len(raw) != len(PrefixService)+43 {
		t.Fatalf("unexpected length %d", len(raw))
	}
	if !bytes.Equal(hash, Hash(raw)) {
		t.Fatal("returned hash does not match Hash(raw)")
	}
	if len(hash) != 32 {
		t.Fatalf("want a SHA-256 hash, got %d bytes", len(hash))
	}
}

func TestNewIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		raw, _ := New(PrefixPersonal)
		if seen[raw] {
			t.Fatal("duplicate token")
		}
		seen[raw] = true
	}
}

func TestRandomStringAlphabet(t *testing.T) {
	s, err := RandomString(500)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(s, "0O1lI") {
		t.Fatalf("contains look-alike characters: %q", s)
	}
	l, err := RandomLower(500)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range l {
		if !strings.ContainsRune(lowerAlnum, r) {
			t.Fatalf("unexpected character %q", r)
		}
	}
}
