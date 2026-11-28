package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type client struct {
	url, token string
	http       *http.Client
}

func newClient(s *settings) *client {
	return &client{url: strings.TrimRight(s.URL, "/"), token: s.Token, http: &http.Client{Timeout: 15 * time.Second}}
}

// apiError is a response from the server; anything else (DNS, refused, timeout) is a transport error.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status) }

// offlineOK says whether err justifies falling back to the cache: the server could not be
// reached or failed (5xx). A definite answer (401 revoked, 403 out of scope, 404) never does.
func offlineOK(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status >= 500
	}
	return err != nil
}

// denied reports a response that means "this token may not have these secrets any more".
func denied(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && (ae.Status == 401 || ae.Status == 403 || ae.Status == 404)
}

func (c *client) do(method, path string, body any) (json.RawMessage, error) {
	if c.token == "" {
		return nil, errors.New("not logged in: run `vaultly login` or set VAULTLY_TOKEN")
	}
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.url+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	var env struct {
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(b, &env)
	if resp.StatusCode >= 300 {
		msg := env.Message
		if msg == "" {
			msg = strings.TrimSpace(string(b))
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, &apiError{Status: resp.StatusCode, Message: msg}
	}
	return env.Data, nil
}

func (c *client) secrets(path string) (map[string]string, error) {
	q := url.Values{"format": {"json"}, "recursive": {"true"}}
	if path != "" {
		q.Set("path", path)
	}
	raw, err := c.do("GET", "/api/v1/secrets?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Secrets == nil {
		out.Secrets = map[string]string{}
	}
	return out.Secrets, nil
}

type whoami struct {
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	Env       string `json:"env"`
	Path      string `json:"path"`
	CanWrite  bool   `json:"can_write"`
}

func (c *client) whoami() (*whoami, error) {
	raw, err := c.do("GET", "/api/v1/whoami", nil)
	if err != nil {
		return nil, err
	}
	w := &whoami{}
	return w, json.Unmarshal(raw, w)
}

// fetchResult says where secrets came from, so the caller can warn about stale data.
type fetchResult struct {
	Secrets  map[string]string
	Cached   bool
	CachedAt time.Time
	Cause    error // why the cache was used
}

// fetch gets secrets from the server, refreshing the cache, and falls back to the cache only
// when offlineOK. Pass a nil cache to disable caching.
func fetch(c *client, path string, ch *cache, maxAge time.Duration, now func() time.Time) (*fetchResult, error) {
	secrets, err := c.secrets(path)
	if err == nil {
		if ch != nil {
			_ = ch.save(secrets, now()) // a cache write failure must not fail the command
		}
		return &fetchResult{Secrets: secrets}, nil
	}
	if ch == nil {
		return nil, err
	}
	if denied(err) {
		ch.drop()
		return nil, err
	}
	if !offlineOK(err) {
		return nil, err
	}
	e, cerr := ch.load(maxAge, now())
	if cerr != nil {
		return nil, fmt.Errorf("%w (and %v)", err, cerr)
	}
	return &fetchResult{Secrets: e.Secrets, Cached: true, CachedAt: e.FetchedAt, Cause: err}, nil
}
