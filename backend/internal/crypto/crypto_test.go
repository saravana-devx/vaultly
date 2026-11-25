package crypto

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

var ctx = context.Background()

func testLocal(t *testing.T) *Local {
	t.Helper()
	b := make([]byte, 32)
	rand.Read(b)
	k, err := NewLocal(base64.StdEncoding.EncodeToString(b))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEnvelopeRoundTrip(t *testing.T) {
	k := testLocal(t)
	dek, wrapped, err := k.NewDEK(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := k.UnwrapDEK(ctx, wrapped)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap mismatch: %v", err)
	}
	ct, _ := Encrypt(dek, []byte("hunter2"), []byte("proj"))
	pt, err := Decrypt(dek, ct, []byte("proj"))
	if err != nil || string(pt) != "hunter2" {
		t.Fatalf("decrypt failed: %v %q", err, pt)
	}
}

func TestTamperAndAAD(t *testing.T) {
	k := testLocal(t)
	dek, _, _ := k.NewDEK(ctx)
	ct, _ := Encrypt(dek, []byte("secret"), []byte("a"))
	if _, err := Decrypt(dek, ct, []byte("b")); err == nil {
		t.Fatal("wrong AAD must fail")
	}
	ct[len(ct)-1] ^= 1
	if _, err := Decrypt(dek, ct, []byte("a")); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
}

func TestWrongKEK(t *testing.T) {
	_, w, _ := testLocal(t).NewDEK(ctx)
	if _, err := testLocal(t).UnwrapDEK(ctx, w); err == nil {
		t.Fatal("different KEK must fail")
	}
}

func TestLocalRewrapKeepsTheSameKey(t *testing.T) {
	k := testLocal(t)
	dek, wrapped, _ := k.NewDEK(ctx)
	again, err := k.Rewrap(ctx, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(wrapped, again) {
		t.Fatal("rewrap must use a fresh nonce")
	}
	if got, _ := k.UnwrapDEK(ctx, again); !bytes.Equal(got, dek) {
		t.Fatal("rewrapped key must unwrap to the same DEK")
	}
}
