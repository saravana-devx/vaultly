package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

func fixedNow() time.Time { return t0 }

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c, _ := newCache(dir, "http://x", "vst_token_a", "/app")
	if err := c.save(map[string]string{"DB": "postgres://secret"}, t0); err != nil {
		t.Fatal(err)
	}
	e, err := c.load(time.Hour, t0.Add(time.Minute))
	if err != nil || e.Secrets["DB"] != "postgres://secret" || !e.FetchedAt.Equal(t0) {
		t.Fatalf("load: %+v %v", e, err)
	}
	st, _ := os.Stat(c.file())
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("cache file mode %v, want 0600", st.Mode().Perm())
	}
	raw, _ := os.ReadFile(c.file())
	if bytes.Contains(raw, []byte("postgres://secret")) || bytes.Contains(raw, []byte("DB")) {
		t.Fatal("cache file is not encrypted")
	}
	if strings.Contains(c.name, "vst_") || strings.Contains(c.name, "app") {
		t.Fatalf("file name leaks inputs: %s", c.name)
	}
}

func TestCacheNeedsSameToken(t *testing.T) {
	dir := t.TempDir()
	a, _ := newCache(dir, "http://x", "vst_token_a", "/")
	a.save(map[string]string{"K": "v"}, t0)

	// Another token maps to a different file; and even pointed at the same file it can't decrypt.
	b, _ := newCache(dir, "http://x", "vst_token_b", "/")
	if b.name == a.name {
		t.Fatal("different tokens must use different cache files")
	}
	b.name, b.label = a.name, a.label
	if _, err := b.load(time.Hour, t0); !errors.Is(err, errCacheMiss) {
		t.Fatalf("wrong token decrypted the cache: %v", err)
	}
}

func TestCacheDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	c, _ := newCache(dir, "http://x", "vst_t", "/")
	c.save(map[string]string{"K": "v"}, t0)
	raw, _ := os.ReadFile(c.file())
	raw[len(raw)-1] ^= 1
	os.WriteFile(c.file(), raw, 0o600)
	if _, err := c.load(time.Hour, t0); !errors.Is(err, errCacheMiss) {
		t.Fatalf("tampered cache accepted: %v", err)
	}
}

func TestCacheMaxAge(t *testing.T) {
	c, _ := newCache(t.TempDir(), "http://x", "vst_t", "/")
	c.save(map[string]string{"K": "v"}, t0)
	if _, err := c.load(time.Hour, t0.Add(59*time.Minute)); err != nil {
		t.Fatalf("fresh entry rejected: %v", err)
	}
	if _, err := c.load(time.Hour, t0.Add(61*time.Minute)); !errors.Is(err, errCacheMiss) {
		t.Fatal("entry older than max-age was used")
	}
	if _, err := c.load(0, t0); !errors.Is(err, errCacheMiss) {
		t.Fatal("max-age 0 must disable the cache")
	}
}

// server returns a fake Vaultly whose /api/v1/secrets answers with status (and secrets on 200).
func server(t *testing.T, status *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vst_ok" {
			w.WriteHeader(401)
			w.Write([]byte(`{"message":"Invalid or expired token"}`))
			return
		}
		w.WriteHeader(*status)
		if *status == 200 {
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"secrets": map[string]string{"DB": "live"}}})
		} else {
			w.Write([]byte(`{"message":"boom"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchFallsBackOnlyWhenOffline(t *testing.T) {
	status := 200
	srv := server(t, &status)
	dir := t.TempDir()
	cl := newClient(&settings{URL: srv.URL, Token: "vst_ok"})
	ch, _ := newCache(dir, srv.URL, "vst_ok", "")

	res, err := fetch(cl, "", ch, time.Hour, fixedNow)
	if err != nil || res.Cached || res.Secrets["DB"] != "live" {
		t.Fatalf("online fetch: %+v %v", res, err)
	}

	status = 503 // server up but failing: cache is fine
	res, err = fetch(cl, "", ch, time.Hour, fixedNow)
	if err != nil || !res.Cached || res.Secrets["DB"] != "live" {
		t.Fatalf("503 should use the cache: %+v %v", res, err)
	}

	srv.Close() // unreachable: cache is fine
	res, err = fetch(cl, "", ch, time.Hour, fixedNow)
	if err != nil || !res.Cached {
		t.Fatalf("offline should use the cache: %+v %v", res, err)
	}
}

func TestFetchNeverFallsBackWhenDenied(t *testing.T) {
	for _, code := range []int{401, 403, 404} {
		status := 200
		srv := server(t, &status)
		dir := t.TempDir()
		cl := newClient(&settings{URL: srv.URL, Token: "vst_ok"})
		ch, _ := newCache(dir, srv.URL, "vst_ok", "")
		if _, err := fetch(cl, "", ch, time.Hour, fixedNow); err != nil {
			t.Fatal(err)
		}
		status = code
		if _, err := fetch(cl, "", ch, time.Hour, fixedNow); err == nil {
			t.Fatalf("HTTP %d: fell back to cache", code)
		}
		if _, err := os.Stat(ch.file()); !os.IsNotExist(err) {
			t.Fatalf("HTTP %d: cache entry should be deleted", code)
		}
	}
}

func TestFetchRevokedToken(t *testing.T) {
	status := 200
	srv := server(t, &status)
	dir := t.TempDir()
	// Seed a cache for a token, then the server stops accepting it.
	ch, _ := newCache(dir, srv.URL, "vst_revoked", "")
	ch.save(map[string]string{"DB": "old"}, t0)
	cl := newClient(&settings{URL: srv.URL, Token: "vst_revoked"})
	_, err := fetch(cl, "", ch, time.Hour, fixedNow)
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 401 {
		t.Fatalf("want 401, got %v", err)
	}
}

func TestFetchWithoutCache(t *testing.T) {
	status := 503
	srv := server(t, &status)
	cl := newClient(&settings{URL: srv.URL, Token: "vst_ok"})
	if _, err := fetch(cl, "", nil, time.Hour, fixedNow); err == nil {
		t.Fatal("no cache: error must surface")
	}
}

func TestResolvePrecedence(t *testing.T) {
	cfg, proj := t.TempDir(), t.TempDir()
	t.Setenv("VAULTLY_CONFIG_DIR", cfg)
	t.Setenv("VAULTLY_TOKEN", "")
	t.Setenv("VAULTLY_URL", "")
	saveCredentials(&Credentials{Profiles: map[string]Profile{
		"default": {URL: "http://default", Token: "vst_default"},
		"ci":      {URL: "http://ci", Token: "vst_ci"},
	}})
	sub := filepath.Join(proj, "a", "b")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(proj, projectFile), []byte(`{"profile":"ci","path":"/app"}`), 0o644)
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	os.Chdir(sub)

	s, err := resolve("", "", "", "")
	if err != nil || s.Token != "vst_ci" || s.URL != "http://ci" || s.Path != "/app" {
		t.Fatalf(".vaultly.json found from a subfolder: %+v %v", s, err)
	}
	if s, _ := resolve("default", "", "", "/other"); s.Token != "vst_default" || s.Path != "/other" {
		t.Fatalf("flags override project file: %+v", s)
	}
	t.Setenv("VAULTLY_TOKEN", "vst_env")
	if s, _ := resolve("", "", "", ""); s.Token != "vst_env" || s.Source != "VAULTLY_TOKEN" {
		t.Fatalf("env overrides profile: %+v", s)
	}
	if s, _ := resolve("", "", "vst_flag", ""); s.Token != "vst_flag" {
		t.Fatalf("--token overrides env: %+v", s)
	}
}

func TestCredentialsFileMode(t *testing.T) {
	t.Setenv("VAULTLY_CONFIG_DIR", t.TempDir())
	if err := saveCredentials(&Credentials{Profiles: map[string]Profile{"default": {Token: "vst_x"}}}); err != nil {
		t.Fatal(err)
	}
	p, _ := credentialsPath()
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode %v, want 0600", st.Mode().Perm())
	}
	c, _ := loadCredentials()
	if c.Profiles["default"].Token != "vst_x" {
		t.Fatal("round trip failed")
	}
}
