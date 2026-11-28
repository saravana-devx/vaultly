// Package demo seeds a fresh stack with a small, realistic workspace for reviewers.
// It drives the public HTTP API, exactly as a user would, so every value is encrypted,
// versioned and audited by the normal code paths. Run it with `server seed-demo`.
package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	AdminEmail = "maya@demo.vaultly.dev"
	DevEmail   = "sam@demo.vaultly.dev"
	Password   = "demo-password-2026"
)

type session struct {
	base, token string
	http        *http.Client
}

type apiErr struct {
	status int
	msg    string
}

func (e *apiErr) Error() string { return fmt.Sprintf("%s (HTTP %d)", e.msg, e.status) }

func (s *session) call(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.base+"/api"+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var env struct {
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode >= 300 {
		return &apiErr{resp.StatusCode, env.Message}
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

func (s *session) auth(path string, body any) error {
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := s.call("POST", path, body, &out); err != nil {
		return err
	}
	s.token = out.AccessToken
	return nil
}

// Result is what the seeder created, printed for the reviewer.
type Result struct {
	AlreadySeeded bool
	ServiceToken  string
}

// Seed creates the demo workspace. It is idempotent: if the demo admin can already sign in,
// it changes nothing.
func Seed(base string, logf func(string, ...any)) (*Result, error) {
	base = strings.TrimRight(base, "/")
	hc := &http.Client{Timeout: 20 * time.Second}
	maya := &session{base: base, http: hc}
	if err := waitHealthy(hc, base); err != nil {
		return nil, err
	}
	if err := maya.auth("/auth/login", map[string]string{"email": AdminEmail, "password": Password}); err == nil {
		return &Result{AlreadySeeded: true}, nil
	}
	logf("creating users")
	if err := maya.auth("/auth/register", map[string]string{"name": "Maya Patel", "email": AdminEmail, "password": Password, "org_name": "Acme"}); err != nil {
		return nil, fmt.Errorf("register admin: %w", err)
	}
	sam := &session{base: base, http: hc}
	if err := sam.auth("/auth/register", map[string]string{"name": "Sam Lee", "email": DevEmail, "password": Password}); err != nil {
		return nil, fmt.Errorf("register developer: %w", err)
	}

	var orgs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := maya.call("GET", "/orgs", nil, &orgs); err != nil || len(orgs) == 0 {
		return nil, fmt.Errorf("list workspaces: %v", err)
	}
	org := orgs[0].ID
	if err := maya.call("POST", "/orgs/"+org+"/members", map[string]string{"email": DevEmail, "role": "developer"}, nil); err != nil {
		return nil, fmt.Errorf("add developer: %w", err)
	}

	logf("creating projects and secrets")
	type env struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	var proj struct {
		ID           string `json:"id"`
		Environments []env  `json:"environments"`
	}
	if err := maya.call("POST", "/orgs/"+org+"/projects", map[string]string{"name": "payments-api"}, &proj); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	put := func(s *session, envSlug, path, key, value, comment string) error {
		return s.call("PUT", "/projects/"+proj.ID+"/secrets", map[string]any{"env": envSlug, "path": path, "key": key, "value": value, "comment": comment}, nil)
	}
	type sec struct{ path, key, comment string }
	// Values per environment; "" means the key is deliberately missing there (visible in Compare).
	data := []struct {
		sec
		dev, staging, prod string
	}{
		{sec{"/", "DB_HOST", ""}, "localhost", "db.staging.internal", "db.prod.internal"},
		{sec{"/", "DB_PASSWORD", "rotated quarterly"}, "dev-only-password", "st4g1ng-Qm9vYmFy", "pr0d-8Hq2xLw9vKz3"},
		{sec{"/", "DATABASE_URL", "built from the two keys above"}, "postgres://app:${DB_PASSWORD}@${DB_HOST}:5432/payments", "postgres://app:${DB_PASSWORD}@${DB_HOST}:5432/payments", "postgres://app:${DB_PASSWORD}@${DB_HOST}:5432/payments"},
		{sec{"/", "REDIS_URL", ""}, "redis://localhost:6379/0", "redis://cache.staging.internal:6379/0", "redis://cache.prod.internal:6379/0"},
		{sec{"/", "LOG_LEVEL", ""}, "debug", "info", "info"},
		{sec{"/", "FEATURE_NEW_CHECKOUT", "rolling out"}, "true", "true", ""},
		{sec{"/", "SENTRY_DSN", ""}, "", "https://abc123@o1.ingest.sentry.io/42", "https://abc123@o1.ingest.sentry.io/42"},
		{sec{"/stripe", "STRIPE_SECRET_KEY", ""}, "sk_test_51Hdemo0000000000000000", "sk_test_51Hdemo0000000000000000", "sk_live_51Hdemo9999999999999999"},
		{sec{"/stripe", "STRIPE_WEBHOOK_SECRET", ""}, "whsec_dev_demo", "whsec_staging_demo", "whsec_prod_demo"},
	}
	for _, d := range data {
		for envSlug, v := range map[string]string{"dev": d.dev, "staging": d.staging, "prod": d.prod} {
			if v == "" {
				continue
			}
			if err := put(maya, envSlug, d.path, d.key, v, d.comment); err != nil {
				return nil, fmt.Errorf("put %s %s: %w", envSlug, d.key, err)
			}
		}
	}
	// A second version, so History and Rollback have something to show.
	if err := put(maya, "prod", "/", "LOG_LEVEL", "warn", "less noise in prod"); err != nil {
		return nil, err
	}

	var other struct{ ID string }
	if err := maya.call("POST", "/orgs/"+org+"/projects", map[string]string{"name": "marketing-site"}, &other); err != nil {
		return nil, err
	}
	for k, v := range map[string]string{"NEXT_PUBLIC_API_URL": "https://api.acme.dev", "CMS_TOKEN": "cms_demo_token_123"} {
		if err := maya.call("PUT", "/projects/"+other.ID+"/secrets", map[string]any{"env": "prod", "key": k, "value": v}, nil); err != nil {
			return nil, err
		}
	}

	logf("protecting production and opening a change request")
	for _, e := range proj.Environments {
		if e.Slug == "prod" {
			if err := maya.call("PATCH", "/projects/"+proj.ID+"/environments/"+e.ID, map[string]bool{"protected": true}, nil); err != nil {
				return nil, fmt.Errorf("protect prod: %w", err)
			}
		}
	}
	// Sam (developer) proposes a production change: it waits for Maya's approval.
	if err := put(sam, "prod", "/", "FEATURE_NEW_CHECKOUT", "true", "enable new checkout in prod"); err != nil {
		return nil, fmt.Errorf("change request: %w", err) // 202 Accepted is success
	}

	logf("creating a read-only service token for production")
	var tok struct {
		Token string `json:"token"`
	}
	if err := maya.call("POST", "/projects/"+proj.ID+"/tokens", map[string]any{"name": "demo-cli", "env": "prod", "path": "/", "expire_days": 30}, &tok); err != nil {
		return nil, fmt.Errorf("service token: %w", err)
	}
	return &Result{ServiceToken: tok.Token}, nil
}

func waitHealthy(hc *http.Client, base string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		resp, err := hc.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("API at %s not healthy: %v", base, err)
		}
		time.Sleep(2 * time.Second)
	}
}
