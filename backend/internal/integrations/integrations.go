package integrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/nacl/box"
)

type Result struct {
	Pushed  int
	Skipped []string
}

type Provider interface {
	Validate(cfg map[string]string) error
	Push(ctx context.Context, cfg map[string]string, secrets map[string]string) (Result, error)
	// Public returns the config fields that are safe to show back to the user.
	Public(cfg map[string]string) map[string]string
}

func Registry(client *http.Client) map[string]Provider {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return map[string]Provider{
		"github":  &GitHub{Client: client, Base: "https://api.github.com"},
		"vercel":  &Vercel{Client: client, Base: "https://api.vercel.com"},
		"railway": &Railway{Client: client, Base: "https://backboard.railway.app"},
	}
}

func require(cfg map[string]string, keys ...string) error {
	for _, k := range keys {
		if strings.TrimSpace(cfg[k]) == "" {
			return fmt.Errorf("%s is required", k)
		}
	}
	return nil
}

func pick(cfg map[string]string, keys ...string) map[string]string {
	out := map[string]string{}
	for _, k := range keys {
		if cfg[k] != "" {
			out[k] = cfg[k]
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func do(client *http.Client, req *http.Request, okCodes ...int) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	for _, c := range okCodes {
		if resp.StatusCode == c {
			return body, nil
		}
	}
	var e struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &e)
	msg := e.Message
	if msg == "" {
		msg = e.Error.Message
	}
	if msg == "" {
		msg = strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
	}
	return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
}

// ---- GitHub Actions secrets ----

type GitHub struct {
	Client *http.Client
	Base   string
}

var (
	repoRe       = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	githubNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (g *GitHub) Validate(cfg map[string]string) error {
	if err := require(cfg, "token", "repo"); err != nil {
		return err
	}
	if !repoRe.MatchString(cfg["repo"]) {
		return fmt.Errorf("repo must look like owner/name")
	}
	return nil
}

func (g *GitHub) Public(cfg map[string]string) map[string]string {
	return pick(cfg, "repo", "environment")
}

func (g *GitHub) req(ctx context.Context, cfg map[string]string, method, path string, body any) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, g.Base+path, rd)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+cfg["token"])
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	r.Header.Set("User-Agent", "Vaultly")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r, nil
}

func (g *GitHub) Push(ctx context.Context, cfg map[string]string, secrets map[string]string) (Result, error) {
	base := "/repos/" + cfg["repo"]
	if env := cfg["environment"]; env != "" {
		base += "/environments/" + url.PathEscape(env)
	}
	base += "/actions/secrets"

	req, err := g.req(ctx, cfg, "GET", base+"/public-key", nil)
	if err != nil {
		return Result{}, err
	}
	raw, err := do(g.Client, req, 200)
	if err != nil {
		return Result{}, fmt.Errorf("fetch public key: %w", err)
	}
	var pk struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(raw, &pk); err != nil {
		return Result{}, err
	}
	keyBytes, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil || len(keyBytes) != 32 {
		return Result{}, fmt.Errorf("unexpected public key from GitHub")
	}
	var pub [32]byte
	copy(pub[:], keyBytes)

	res := Result{}
	for _, k := range sortedKeys(secrets) {
		name := strings.ToUpper(k)
		if !githubNameRe.MatchString(name) || strings.HasPrefix(name, "GITHUB_") {
			res.Skipped = append(res.Skipped, k)
			continue
		}
		sealed, err := box.SealAnonymous(nil, []byte(secrets[k]), &pub, rand.Reader)
		if err != nil {
			return res, err
		}
		put, err := g.req(ctx, cfg, "PUT", base+"/"+name, map[string]string{"encrypted_value": base64.StdEncoding.EncodeToString(sealed), "key_id": pk.KeyID})
		if err != nil {
			return res, err
		}
		if _, err := do(g.Client, put, 201, 204); err != nil {
			return res, fmt.Errorf("%s: %w", name, err)
		}
		res.Pushed++
	}
	return res, nil
}

// ---- Vercel environment variables ----

type Vercel struct {
	Client *http.Client
	Base   string
}

func (v *Vercel) Validate(cfg map[string]string) error {
	if err := require(cfg, "token", "project_id"); err != nil {
		return err
	}
	for _, t := range v.targets(cfg) {
		if t != "production" && t != "preview" && t != "development" {
			return fmt.Errorf("target must be production, preview and/or development")
		}
	}
	return nil
}

func (v *Vercel) Public(cfg map[string]string) map[string]string {
	return pick(cfg, "project_id", "team_id", "target")
}

func (v *Vercel) targets(cfg map[string]string) []string {
	if strings.TrimSpace(cfg["target"]) == "" {
		return []string{"production"}
	}
	var out []string
	for _, t := range strings.Split(cfg["target"], ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (v *Vercel) Push(ctx context.Context, cfg map[string]string, secrets map[string]string) (Result, error) {
	type env struct {
		Key    string   `json:"key"`
		Value  string   `json:"value"`
		Type   string   `json:"type"`
		Target []string `json:"target"`
	}
	targets := v.targets(cfg)
	var all []env
	for _, k := range sortedKeys(secrets) {
		all = append(all, env{Key: k, Value: secrets[k], Type: "encrypted", Target: targets})
	}
	res := Result{}
	for start := 0; start < len(all); start += 100 {
		end := min(start+100, len(all))
		q := url.Values{"upsert": {"true"}}
		if cfg["team_id"] != "" {
			q.Set("teamId", cfg["team_id"])
		}
		b, _ := json.Marshal(all[start:end])
		req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/v10/projects/%s/env?%s", v.Base, url.PathEscape(cfg["project_id"]), q.Encode()), bytes.NewReader(b))
		if err != nil {
			return res, err
		}
		req.Header.Set("Authorization", "Bearer "+cfg["token"])
		req.Header.Set("Content-Type", "application/json")
		if _, err := do(v.Client, req, 200, 201); err != nil {
			return res, err
		}
		res.Pushed += end - start
	}
	return res, nil
}

// ---- Railway variables ----

type Railway struct {
	Client *http.Client
	Base   string
}

func (r *Railway) Validate(cfg map[string]string) error {
	return require(cfg, "token", "project_id", "environment_id")
}

func (r *Railway) Public(cfg map[string]string) map[string]string {
	return pick(cfg, "project_id", "environment_id", "service_id")
}

func (r *Railway) Push(ctx context.Context, cfg map[string]string, secrets map[string]string) (Result, error) {
	input := map[string]any{"projectId": cfg["project_id"], "environmentId": cfg["environment_id"], "variables": secrets}
	if cfg["service_id"] != "" {
		input["serviceId"] = cfg["service_id"]
	}
	body, _ := json.Marshal(map[string]any{
		"query":     `mutation($input: VariableCollectionUpsertInput!) { variableCollectionUpsert(input: $input) }`,
		"variables": map[string]any{"input": input},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", r.Base+"/graphql/v2", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg["token"])
	req.Header.Set("Content-Type", "application/json")
	raw, err := do(r.Client, req, 200)
	if err != nil {
		return Result{}, err
	}
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	json.Unmarshal(raw, &out)
	if len(out.Errors) > 0 {
		return Result{}, fmt.Errorf("%s", out.Errors[0].Message)
	}
	return Result{Pushed: len(secrets)}, nil
}

var (
	_ Provider = (*GitHub)(nil)
	_ Provider = (*Vercel)(nil)
	_ Provider = (*Railway)(nil)
)
