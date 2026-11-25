package backup

import (
	"bytes"
	"errors"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	plain := bytes.Repeat([]byte(`{"projects":[]}`), 1000)
	blob, err := Seal("correct horse battery", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte("projects")) {
		t.Fatal("ciphertext must not contain plaintext")
	}
	got, err := Open("correct horse battery", blob)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip failed: %v", err)
	}
}

func TestWrongPassphraseAndTamper(t *testing.T) {
	blob, _ := Seal("correct horse battery", []byte("data"))
	if _, err := Open("another passphrase!!", blob); !errors.Is(err, ErrBadPassphrase) {
		t.Fatalf("expected ErrBadPassphrase, got %v", err)
	}
	blob[len(blob)-1] ^= 1
	if _, err := Open("correct horse battery", blob); !errors.Is(err, ErrBadPassphrase) {
		t.Fatalf("tampered file must fail, got %v", err)
	}
}

func TestRejectsWeakPassphraseAndForeignFiles(t *testing.T) {
	if _, err := Seal("short", []byte("x")); err == nil {
		t.Fatal("short passphrase must be rejected")
	}
	if _, err := Open("whatever pass phrase", []byte("not a backup file at all, definitely")); err == nil {
		t.Fatal("foreign file must be rejected")
	}
}

func TestSealIsRandomised(t *testing.T) {
	a, _ := Seal("correct horse battery", []byte("same"))
	b, _ := Seal("correct horse battery", []byte("same"))
	if bytes.Equal(a, b) {
		t.Fatal("salt and nonce must differ between backups")
	}
}
