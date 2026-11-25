package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeBao imitates the parts of the Transit API Vaultly uses. Ciphertexts are
// AES-GCM under a key that exists only inside the fake, like the real engine.
type fakeBao struct {
	key     []byte
	version int32
	calls   int32
}

func (f *fakeBao) seal(plain []byte) string {
	ct, _ := Encrypt(f.key, plain, nil)
	return fmt.Sprintf("vault:v%d:%s", atomic.LoadInt32(&f.version), base64.StdEncoding.EncodeToString(ct))
}

func (f *fakeBao) open(s string) ([]byte, bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, false
	}
	p, err := Decrypt(f.key, raw, nil)
	return p, err == nil
}

func (f *fakeBao) handler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&f.calls, 1)
	if r.Header.Get("X-Vault-Token") != "s.good" {
		w.WriteHeader(403)
		w.Write([]byte(`{"errors":["permission denied"]}`))
		return
	}
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	out := map[string]string{}
	switch {
	case strings.HasPrefix(r.URL.Path, "/v1/transit/datakey/plaintext/vaultly"):
		p := make([]byte, 32)
		rand.Read(p)
		out["plaintext"], out["ciphertext"] = base64.StdEncoding.EncodeToString(p), f.seal(p)
	case r.URL.Path == "/v1/transit/encrypt/vaultly":
		p, _ := base64.StdEncoding.DecodeString(in["plaintext"].(string))
		out["ciphertext"] = f.seal(p)
	case r.URL.Path == "/v1/transit/decrypt/vaultly":
		p, ok := f.open(in["ciphertext"].(string))
		if !ok {
			w.WriteHeader(400)
			w.Write([]byte(`{"errors":["cipher: message authentication failed"]}`))
			return
		}
		out["plaintext"] = base64.StdEncoding.EncodeToString(p)
	case r.URL.Path == "/v1/transit/rewrap/vaultly":
		p, ok := f.open(in["ciphertext"].(string))
		if !ok {
			w.WriteHeader(400)
			return
		}
		out["ciphertext"] = f.seal(p)
	default:
		w.WriteHeader(404)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

func newFake(t *testing.T) (*fakeBao, *Transit) {
	t.Helper()
	f := &fakeBao{key: make([]byte, 32), version: 1}
	rand.Read(f.key)
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return f, NewTransit(srv.URL, "transit", "vaultly", func() (string, error) { return "s.good", nil }, srv.Client())
}

func TestTransitDataKeyRoundTrip(t *testing.T) {
	_, tr := newFake(t)
	plain, wrapped, err := tr.NewDEK(ctx)
	if err != nil || len(plain) != 32 || !IsTransit(wrapped) {
		t.Fatalf("NewDEK: %v %q", err, wrapped)
	}
	got, err := tr.UnwrapDEK(ctx, wrapped)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	if err := tr.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTransitErrorsAreReadableAndHideTheToken(t *testing.T) {
	f := &fakeBao{key: make([]byte, 32), version: 1}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()
	bad := NewTransit(srv.URL, "transit", "vaultly", func() (string, error) { return "s.WRONG-TOKEN", nil }, srv.Client())
	_, _, err := bad.NewDEK(ctx)
	if err == nil || !strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "WRONG-TOKEN") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := bad.UnwrapDEK(ctx, []byte("not-transit")); err == nil {
		t.Fatal("non-transit input must be rejected before any network call")
	}
}

func TestTransitRewrapMovesToNewKeyVersionWithoutChangingTheDEK(t *testing.T) {
	f, tr := newFake(t)
	plain, wrapped, _ := tr.NewDEK(ctx)
	atomic.StoreInt32(&f.version, 2)
	again, err := tr.Rewrap(ctx, wrapped)
	if err != nil || !strings.HasPrefix(string(again), "vault:v2:") {
		t.Fatalf("rewrap: %v %q", err, again)
	}
	if got, _ := tr.UnwrapDEK(ctx, again); !bytes.Equal(got, plain) {
		t.Fatal("DEK changed during rewrap")
	}
}

func TestHybridReadsLegacyKeysAndMigratesThem(t *testing.T) {
	_, tr := newFake(t)
	local := testLocal(t)
	h := &Hybrid{Primary: tr, Legacy: local}

	legacyDEK, legacyWrapped, _ := local.NewDEK(ctx)
	if got, err := h.UnwrapDEK(ctx, legacyWrapped); err != nil || !bytes.Equal(got, legacyDEK) {
		t.Fatalf("legacy key must still unwrap: %v", err)
	}
	_, fresh, _ := h.NewDEK(ctx)
	if !IsTransit(fresh) {
		t.Fatal("new keys must be wrapped by OpenBao")
	}
	migrated, err := h.Rewrap(ctx, legacyWrapped)
	if err != nil || !IsTransit(migrated) {
		t.Fatalf("rewrap of a legacy key must move it to OpenBao: %v", err)
	}
	if got, _ := tr.UnwrapDEK(ctx, migrated); !bytes.Equal(got, legacyDEK) {
		t.Fatal("migration changed the DEK - existing ciphertext would become unreadable")
	}
}

func TestHybridWithoutLegacyKeyGivesAClearError(t *testing.T) {
	_, tr := newFake(t)
	_, w, _ := testLocal(t).NewDEK(ctx)
	_, err := (&Hybrid{Primary: tr}).UnwrapDEK(ctx, w)
	if err == nil || !strings.Contains(err.Error(), "rewrap") {
		t.Fatalf("expected guidance about MASTER_KEY/rewrap, got %v", err)
	}
}
