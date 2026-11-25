package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/audit"
)

func event() audit.Event {
	pid := uuid.New()
	return audit.Event{ID: uuid.New(), OrgID: uuid.New(), ProjectID: &pid, ActorType: "user", ActorLabel: "a@b.c", Action: "secret.update", Resource: "/KEY", At: time.Now(), Metadata: map[string]any{"env": "prod"}}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pats   []string
		action string
		want   bool
	}{
		{[]string{"*"}, "secret.update", true},
		{[]string{"secret.*"}, "secret.update", true},
		{[]string{"secret.*"}, "token.create", false},
		{[]string{"token.create", "secret.delete"}, "secret.delete", true},
		{nil, "secret.update", false},
	}
	for _, c := range cases {
		if got := Match(c.pats, c.action); got != c.want {
			t.Errorf("Match(%v,%q)=%v want %v", c.pats, c.action, got, c.want)
		}
	}
}

func TestSignedDeliveryAndNoSecretValues(t *testing.T) {
	var gotSig, gotTS string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig, gotTS = r.Header.Get("X-Vaultly-Signature"), r.Header.Get("X-Vaultly-Timestamp")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	d := &Dispatcher{client: NewClient(true)}
	res := d.Deliver(context.Background(), Hook{URL: srv.URL, Secret: "whsec_test"}, event(), "payments", false)
	if res.Error != "" || res.Status != 204 {
		t.Fatalf("delivery failed: %+v", res)
	}
	if want := Sign("whsec_test", gotTS, body); gotSig != want {
		t.Fatalf("signature mismatch: %s vs %s", gotSig, want)
	}
	var p map[string]any
	json.Unmarshal(body, &p)
	if p["event"] != "secret.update" || p["project"] != "payments" {
		t.Fatalf("unexpected payload: %s", body)
	}
	if _, has := p["value"]; has {
		t.Fatal("payload must never contain secret values")
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	d := &Dispatcher{client: NewClient(true), delays: []time.Duration{5 * time.Millisecond, 5 * time.Millisecond}}
	res := d.Deliver(context.Background(), Hook{URL: srv.URL, Secret: "s"}, event(), "p", true)
	if res.Error != "" || res.Attempts != 3 {
		t.Fatalf("expected success on 3rd attempt, got %+v", res)
	}
}

func TestPrivateAddressesBlockedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	d := &Dispatcher{client: NewClient(false)}
	res := d.Deliver(context.Background(), Hook{URL: srv.URL, Secret: "s"}, event(), "p", false)
	if res.Error == "" || res.Status != 0 {
		t.Fatalf("loopback destination must be refused, got %+v", res)
	}
}

func TestRedirectsNotFollowed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect was followed") }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer srv.Close()
	d := &Dispatcher{client: NewClient(true)}
	res := d.Deliver(context.Background(), Hook{URL: srv.URL, Secret: "s"}, event(), "p", false)
	if res.Error == "" || res.Status != 302 {
		t.Fatalf("expected non-2xx result for redirect, got %+v", res)
	}
}
