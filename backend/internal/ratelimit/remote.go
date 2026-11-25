package ratelimit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Remote asks the standalone Distributed-rate-limiter service for each decision. Its policies
// live per client (algorithm, limit, window), so the first request for a given (limit, window)
// registers a client named "vaultly-<limit>-<seconds>" and caches its API key. If the service
// is unreachable the decision falls back to the in-process limiter.
type Remote struct {
	baseURL  string
	http     *http.Client
	fallback Limiter

	mu   sync.Mutex
	keys map[string]string
}

func NewRemote(baseURL string, fallback Limiter) *Remote {
	return &Remote{baseURL: baseURL, http: &http.Client{Timeout: 2 * time.Second}, fallback: fallback, keys: map[string]string{}}
}

type envelope struct {
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (r *Remote) Allow(ctx context.Context, key string, limit int, window time.Duration) (*Result, error) {
	res, err := r.check(ctx, key, limit, window)
	if err != nil {
		log.Printf("ratelimiter service unavailable (%v), using local limiter", err)
		return r.fallback.Allow(ctx, key, limit, window)
	}
	return res, nil
}

func (r *Remote) check(ctx context.Context, identifier string, limit int, window time.Duration) (*Result, error) {
	seconds := int(window.Seconds())
	apiKey, err := r.apiKey(ctx, limit, seconds)
	if err != nil {
		return nil, err
	}
	var out struct {
		Allowed   bool  `json:"allowed"`
		Remaining int64 `json:"remaining"`
		ResetAt   int64 `json:"reset_at"`
	}
	status, err := r.do(ctx, http.MethodPost, "/api/v1/check", apiKey, map[string]string{"identifier": identifier}, &out)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK && status != http.StatusTooManyRequests {
		return nil, fmt.Errorf("check returned %d", status)
	}
	return &Result{Allowed: out.Allowed, Remaining: out.Remaining, ResetAt: time.Unix(out.ResetAt, 0)}, nil
}

func (r *Remote) apiKey(ctx context.Context, limit, seconds int) (string, error) {
	name := fmt.Sprintf("vaultly-%d-%d", limit, seconds)
	r.mu.Lock()
	defer r.mu.Unlock()
	if k, ok := r.keys[name]; ok {
		return k, nil
	}
	var c struct {
		APIKey string `json:"APIKey"`
	}
	status, err := r.do(ctx, http.MethodPost, "/api/v1/clients", "", map[string]any{
		"clientId": name, "algorithm": "sliding_window", "limit": limit, "window_seconds": seconds,
	}, &c)
	if err != nil {
		return "", err
	}
	if status == http.StatusConflict {
		if status, err = r.do(ctx, http.MethodGet, "/api/v1/clients/"+name, "", nil, &c); err != nil {
			return "", err
		}
	}
	if (status != http.StatusCreated && status != http.StatusOK) || c.APIKey == "" {
		return "", fmt.Errorf("could not register policy %s (status %d)", name, status)
	}
	r.keys[name] = c.APIKey
	return c.APIKey, nil
}

// do sends one request and decodes the envelope's data field into out.
func (r *Remote) do(ctx context.Context, method, path, apiKey string, body any, out any) (int, error) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err == nil && len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, out)
	}
	return resp.StatusCode, nil
}
