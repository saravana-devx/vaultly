package crypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const transitPrefix = "vault:v"

// IsTransit reports whether a wrapped key was produced by a Transit engine.
func IsTransit(wrapped []byte) bool { return bytes.HasPrefix(wrapped, []byte(transitPrefix)) }

// Transit wraps data keys with a key that never leaves OpenBao (or HashiCorp Vault).
// Only the key-wrapping calls are made; secret values are still encrypted locally.
type Transit struct {
	addr, mount, key string
	token            func() (string, error)
	client           *http.Client
}

func NewTransit(addr, mount, key string, token func() (string, error), client *http.Client) *Transit {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Transit{addr: strings.TrimRight(addr, "/"), mount: strings.Trim(mount, "/"), key: key, token: token, client: client}
}

func (t *Transit) Name() string { return "openbao" }

func (t *Transit) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.addr+"/v1/"+path, body)
	if err != nil {
		return err
	}
	tok, err := t.token()
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("openbao unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Errors []string `json:"errors"`
		}
		json.Unmarshal(raw, &e)
		msg := strings.Join(e.Errors, "; ")
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("openbao %s: %s (HTTP %d)", path, msg, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

type transitData struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
		Plaintext  string `json:"plaintext"`
	} `json:"data"`
}

func (t *Transit) NewDEK(ctx context.Context) (plain, wrapped []byte, err error) {
	var r transitData
	if err = t.call(ctx, http.MethodPost, t.mount+"/datakey/plaintext/"+t.key, map[string]int{"bits": 256}, &r); err != nil {
		return nil, nil, err
	}
	plain, err = base64.StdEncoding.DecodeString(r.Data.Plaintext)
	if err != nil || len(plain) != 32 || !strings.HasPrefix(r.Data.Ciphertext, transitPrefix) {
		return nil, nil, errors.New("openbao returned an unexpected data key")
	}
	return plain, []byte(r.Data.Ciphertext), nil
}

// Wrap protects an existing key (used when migrating from the local master key).
func (t *Transit) Wrap(ctx context.Context, plain []byte) ([]byte, error) {
	var r transitData
	if err := t.call(ctx, http.MethodPost, t.mount+"/encrypt/"+t.key, map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plain)}, &r); err != nil {
		return nil, err
	}
	return []byte(r.Data.Ciphertext), nil
}

func (t *Transit) UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error) {
	if !IsTransit(wrapped) {
		return nil, errors.New("key was not wrapped by OpenBao")
	}
	var r transitData
	if err := t.call(ctx, http.MethodPost, t.mount+"/decrypt/"+t.key, map[string]string{"ciphertext": string(wrapped)}, &r); err != nil {
		return nil, err
	}
	plain, err := base64.StdEncoding.DecodeString(r.Data.Plaintext)
	if err != nil || len(plain) != 32 {
		return nil, errors.New("openbao returned an unexpected key")
	}
	return plain, nil
}

// Rewrap moves a wrapped key to the latest key version without exposing it.
func (t *Transit) Rewrap(ctx context.Context, wrapped []byte) ([]byte, error) {
	var r transitData
	if err := t.call(ctx, http.MethodPost, t.mount+"/rewrap/"+t.key, map[string]string{"ciphertext": string(wrapped)}, &r); err != nil {
		return nil, err
	}
	return []byte(r.Data.Ciphertext), nil
}

// Check proves the token and key work: create a key, unwrap it, compare.
func (t *Transit) Check(ctx context.Context) error {
	plain, wrapped, err := t.NewDEK(ctx)
	if err != nil {
		return err
	}
	back, err := t.UnwrapDEK(ctx, wrapped)
	if err != nil || !bytes.Equal(plain, back) {
		return errors.New("openbao round-trip check failed")
	}
	return nil
}

// RunRenewer keeps a periodic token alive until ctx ends.
func (t *Transit) RunRenewer(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := t.call(ctx, http.MethodPost, "auth/token/renew-self", map[string]string{}, nil); err != nil {
				log.Printf("openbao: token renewal failed: %v", err)
			}
		}
	}
}

var _ KeyProvider = (*Transit)(nil)

// Hybrid uses OpenBao for everything new and keeps reading keys that are still
// wrapped by the legacy local master key, until `rewrap` has migrated them.
type Hybrid struct {
	Primary *Transit
	Legacy  *Local // optional
}

func (h *Hybrid) Name() string { return "openbao" }

func (h *Hybrid) NewDEK(ctx context.Context) ([]byte, []byte, error) { return h.Primary.NewDEK(ctx) }

func (h *Hybrid) UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error) {
	if IsTransit(wrapped) {
		return h.Primary.UnwrapDEK(ctx, wrapped)
	}
	if h.Legacy == nil {
		return nil, errors.New("key is wrapped with a local MASTER_KEY that is not configured; set MASTER_KEY and run `server rewrap`")
	}
	return h.Legacy.UnwrapDEK(ctx, wrapped)
}

func (h *Hybrid) Rewrap(ctx context.Context, wrapped []byte) ([]byte, error) {
	if IsTransit(wrapped) {
		return h.Primary.Rewrap(ctx, wrapped)
	}
	plain, err := h.UnwrapDEK(ctx, wrapped)
	if err != nil {
		return nil, err
	}
	return h.Primary.Wrap(ctx, plain)
}

var _ KeyProvider = (*Hybrid)(nil)
