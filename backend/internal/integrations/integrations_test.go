package integrations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestGitHubSealsSecretsForTheRepoKey(t *testing.T) {
	pub, priv, _ := box.GenerateKey(rand.Reader)
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/actions/secrets/public-key"):
			json.NewEncoder(w).Encode(map[string]string{"key_id": "k1", "key": base64.StdEncoding.EncodeToString(pub[:])})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/actions/secrets/"):
			var b struct{ EncryptedValue, KeyID string }
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal([]byte(strings.NewReplacer("encrypted_value", "EncryptedValue", "key_id", "KeyID").Replace(string(body))), &b)
			sealed, _ := base64.StdEncoding.DecodeString(b.EncryptedValue)
			plain, ok := box.OpenAnonymous(nil, sealed, pub, priv)
			if !ok || b.KeyID != "k1" {
				w.WriteHeader(422)
				return
			}
			got[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]] = string(plain)
			w.WriteHeader(201)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	g := &GitHub{Client: srv.Client(), Base: srv.URL}
	res, err := g.Push(context.Background(), map[string]string{"token": "ghp_test", "repo": "acme/api"},
		map[string]string{"db_url": "postgres://x", "STRIPE_KEY": "sk_1", "GITHUB_TOKEN": "nope", "bad-name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 2 || got["DB_URL"] != "postgres://x" || got["STRIPE_KEY"] != "sk_1" {
		t.Fatalf("pushed=%d got=%v", res.Pushed, got)
	}
	if len(res.Skipped) != 2 {
		t.Fatalf("expected GITHUB_* and invalid names skipped, got %v", res.Skipped)
	}
}

func TestGitHubEnvironmentSecretsUseEnvironmentPath(t *testing.T) {
	var paths []string
	pub, _, _ := box.GenerateKey(rand.Reader)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]string{"key_id": "k", "key": base64.StdEncoding.EncodeToString(pub[:])})
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	g := &GitHub{Client: srv.Client(), Base: srv.URL}
	if _, err := g.Push(context.Background(), map[string]string{"token": "t", "repo": "a/b", "environment": "production"}, map[string]string{"K": "v"}); err != nil {
		t.Fatal(err)
	}
	if paths[0] != "/repos/a/b/environments/production/actions/secrets/public-key" {
		t.Fatalf("wrong path %s", paths[0])
	}
}

func TestGitHubSurfacesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()
	g := &GitHub{Client: srv.Client(), Base: srv.URL}
	_, err := g.Push(context.Background(), map[string]string{"token": "t", "repo": "a/b"}, map[string]string{"K": "v"})
	if err == nil || !strings.Contains(err.Error(), "Not Found") {
		t.Fatalf("expected API error, got %v", err)
	}
}

func TestVercelUpsertsWithTargetsAndTeam(t *testing.T) {
	var q string
	var body []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		if r.URL.Path != "/v10/projects/prj_1/env" || r.Header.Get("Authorization") != "Bearer vt" {
			w.WriteHeader(400)
			return
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(201)
	}))
	defer srv.Close()
	v := &Vercel{Client: srv.Client(), Base: srv.URL}
	cfg := map[string]string{"token": "vt", "project_id": "prj_1", "team_id": "team_9", "target": "production, preview"}
	if err := v.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	res, err := v.Push(context.Background(), cfg, map[string]string{"A": "1", "B": "2"})
	if err != nil || res.Pushed != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if !strings.Contains(q, "upsert=true") || !strings.Contains(q, "teamId=team_9") {
		t.Fatalf("query %q", q)
	}
	if len(body) != 2 || body[0]["type"] != "encrypted" || len(body[0]["target"].([]any)) != 2 {
		t.Fatalf("body %v", body)
	}
}

func TestVercelRejectsUnknownTarget(t *testing.T) {
	v := &Vercel{}
	if v.Validate(map[string]string{"token": "t", "project_id": "p", "target": "staging"}) == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestRailwayUpsertsAndReportsGraphQLErrors(t *testing.T) {
	fail := false
	var in map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Variables struct{ Input map[string]any } `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		in = b.Variables.Input
		if fail {
			w.Write([]byte(`{"errors":[{"message":"Not Authorized"}]}`))
			return
		}
		w.Write([]byte(`{"data":{"variableCollectionUpsert":true}}`))
	}))
	defer srv.Close()
	rw := &Railway{Client: srv.Client(), Base: srv.URL}
	cfg := map[string]string{"token": "t", "project_id": "p", "environment_id": "e", "service_id": "s"}
	res, err := rw.Push(context.Background(), cfg, map[string]string{"A": "1"})
	if err != nil || res.Pushed != 1 || in["serviceId"] != "s" {
		t.Fatalf("%+v %v %v", res, err, in)
	}
	fail = true
	if _, err := rw.Push(context.Background(), cfg, map[string]string{"A": "1"}); err == nil || !strings.Contains(err.Error(), "Not Authorized") {
		t.Fatalf("expected graphql error, got %v", err)
	}
}

func TestPublicHidesTokens(t *testing.T) {
	for name, p := range Registry(nil) {
		pub := p.Public(map[string]string{"token": "SECRET", "repo": "a/b", "project_id": "p", "environment_id": "e"})
		for _, v := range pub {
			if v == "SECRET" {
				t.Fatalf("%s leaks token", name)
			}
		}
	}
}
