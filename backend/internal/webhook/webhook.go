package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saravana/vaultly/internal/audit"
	"github.com/saravana/vaultly/internal/crypto"
)

type Hook struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	URL       string
	Secret    string
	Events    []string
}

type Result struct {
	Status   int
	Error    string
	Attempts int
	Duration time.Duration
}

type Dispatcher struct {
	pool   *pgxpool.Pool
	keys   crypto.KeyProvider
	client *http.Client
	delays []time.Duration
}

func New(pool *pgxpool.Pool, keys crypto.KeyProvider, allowPrivate bool) *Dispatcher {
	return &Dispatcher{pool: pool, keys: keys, client: NewClient(allowPrivate), delays: []time.Duration{time.Second, 5 * time.Second}}
}

var errBlocked = errors.New("destination address is not allowed")

func BlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// NewClient returns an HTTP client that refuses redirects and, unless
// allowPrivate is set, refuses to connect to loopback or private addresses.
// The check runs on the resolved address at dial time, so DNS tricks can't bypass it.
func NewClient(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || BlockedIP(ip) {
				return errBlocked
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:       10 * time.Second,
		Transport:     &http.Transport{DialContext: d.DialContext, MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func Sign(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp + "."))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

func Match(patterns []string, action string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, action); ok || p == "*" {
			return true
		}
	}
	return false
}

type payload struct {
	ID        uuid.UUID      `json:"id"`
	Event     string         `json:"event"`
	CreatedAt time.Time      `json:"created_at"`
	ProjectID *uuid.UUID     `json:"project_id,omitempty"`
	Project   string         `json:"project,omitempty"`
	Actor     string         `json:"actor"`
	ActorType string         `json:"actor_type"`
	Resource  string         `json:"resource"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Deliver posts one event to a hook, retrying transient failures.
func (d *Dispatcher) Deliver(ctx context.Context, h Hook, ev audit.Event, projectName string, retries bool) Result {
	body, _ := json.Marshal(payload{ID: ev.ID, Event: ev.Action, CreatedAt: ev.At, ProjectID: ev.ProjectID, Project: projectName,
		Actor: ev.ActorLabel, ActorType: ev.ActorType, Resource: ev.Resource, Metadata: ev.Metadata})
	start := time.Now()
	res := Result{}
	for attempt := 0; ; attempt++ {
		res.Attempts = attempt + 1
		res.Status, res.Error = d.post(ctx, h, ev, body)
		if res.Error == "" || !retries || attempt >= len(d.delays) {
			break
		}
		select {
		case <-ctx.Done():
			res.Error = ctx.Err().Error()
			res.Duration = time.Since(start)
			return res
		case <-time.After(d.delays[attempt]):
		}
	}
	res.Duration = time.Since(start)
	return res
}

func (d *Dispatcher) post(ctx context.Context, h Hook, ev audit.Event, body []byte) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Vaultly-Webhooks/1")
	req.Header.Set("X-Vaultly-Event", ev.Action)
	req.Header.Set("X-Vaultly-Delivery", ev.ID.String())
	req.Header.Set("X-Vaultly-Timestamp", ts)
	req.Header.Set("X-Vaultly-Signature", Sign(h.Secret, ts, body))
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Sprintf("endpoint returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, ""
}

// Handle is an audit subscriber: it fans an event out to the project's matching hooks.
func (d *Dispatcher) Handle(ev audit.Event) {
	if ev.ProjectID == nil || strings.HasPrefix(ev.Action, "webhook.") {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	hooks, name, err := d.hooksFor(ctx, *ev.ProjectID, ev.Action)
	if err != nil {
		log.Printf("webhook: load hooks: %v", err)
		return
	}
	for _, h := range hooks {
		go func(h Hook) {
			dctx, dcancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer dcancel()
			res := d.Deliver(dctx, h, ev, name, true)
			d.record(dctx, h.ID, ev, res)
		}(h)
	}
}

func (d *Dispatcher) hooksFor(ctx context.Context, projectID uuid.UUID, action string) ([]Hook, string, error) {
	var wrapped []byte
	var name string
	if err := d.pool.QueryRow(ctx, `SELECT encrypted_dek, name FROM projects WHERE id=$1`, projectID).Scan(&wrapped, &name); err != nil {
		return nil, "", err
	}
	rows, err := d.pool.Query(ctx, `SELECT id, url, secret_enc, events FROM webhooks WHERE project_id=$1 AND active`, projectID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var dek []byte
	var out []Hook
	for rows.Next() {
		h := Hook{ProjectID: projectID}
		var enc []byte
		if err := rows.Scan(&h.ID, &h.URL, &enc, &h.Events); err != nil || !Match(h.Events, action) {
			continue
		}
		if dek == nil {
			if dek, err = d.keys.UnwrapDEK(ctx, wrapped); err != nil {
				return nil, "", err
			}
		}
		secret, err := crypto.Decrypt(dek, enc, []byte(projectID.String()))
		if err != nil {
			continue
		}
		h.Secret = string(secret)
		out = append(out, h)
	}
	return out, name, rows.Err()
}

func (d *Dispatcher) record(ctx context.Context, hookID uuid.UUID, ev audit.Event, r Result) {
	_, err := d.pool.Exec(ctx, `INSERT INTO webhook_deliveries (webhook_id,event_id,action,status,error,attempts,duration_ms) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		hookID, ev.ID, ev.Action, r.Status, r.Error, r.Attempts, r.Duration.Milliseconds())
	if err != nil {
		log.Printf("webhook: record delivery: %v", err)
	}
	d.pool.Exec(ctx, `DELETE FROM webhook_deliveries WHERE webhook_id=$1 AND id NOT IN (SELECT id FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY id DESC LIMIT 100)`, hookID)
}

func (d *Dispatcher) Record(ctx context.Context, hookID uuid.UUID, ev audit.Event, r Result) {
	d.record(ctx, hookID, ev, r)
}
