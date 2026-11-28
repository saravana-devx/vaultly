package secret

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/crypto"
	"vaultly/internal/keystore"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
)

// ---- in-memory world: one project, envs dev (open) and prod (protected), three members ----

type memAccess struct {
	roles map[uuid.UUID]string // user -> workspace role
	envs  []access.Env
	prot  map[uuid.UUID]bool
	org   uuid.UUID
}

func (m *memAccess) OrgRole(_ context.Context, u, _ uuid.UUID) (string, error) {
	return m.roles[u], nil
}
func (m *memAccess) ProjectWithRole(_ context.Context, u, p uuid.UUID) (*access.Project, error) {
	r, ok := m.roles[u]
	if !ok {
		return nil, access.ErrNotFound
	}
	return &access.Project{ID: p, OrgID: m.org, Role: r}, nil
}
func (m *memAccess) EnvOverride(context.Context, uuid.UUID, uuid.UUID, string) (string, bool, error) {
	return "", false, nil
}
func (m *memAccess) Envs(context.Context, uuid.UUID) ([]access.Env, error) { return m.envs, nil }
func (m *memAccess) EnvID(_ context.Context, _ uuid.UUID, slug string) (uuid.UUID, error) {
	for _, e := range m.envs {
		if e.Slug == slug {
			return e.ID, nil
		}
	}
	return uuid.Nil, access.ErrNotFound
}
func (m *memAccess) EnvProtected(_ context.Context, id uuid.UUID) (bool, error) {
	return m.prot[id], nil
}

type memSecret struct {
	id      uuid.UUID
	env     uuid.UUID
	path    string
	key     string
	version int
	ct      []byte
}

type memStore struct {
	SecretRepository // unused methods panic if called
	mu               sync.Mutex
	envSlug          map[uuid.UUID]string
	secrets          map[string]*memSecret // env|path|key
	changes          map[uuid.UUID]*StoredChange
}

func sk(env uuid.UUID, path, key string) string { return env.String() + "|" + path + "|" + key }

func (m *memStore) Upsert(_ context.Context, w Write) (Secret, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := m.secrets[sk(w.EnvID, w.Path, w.Key)]
	if w.CheckVersion {
		v := 0
		if cur != nil {
			v = cur.version
		}
		if v != w.ExpectVersion {
			return Secret{}, false, ErrStaleChange
		}
	}
	if cur == nil {
		cur = &memSecret{id: uuid.New(), env: w.EnvID, path: w.Path, key: w.Key}
		m.secrets[sk(w.EnvID, w.Path, w.Key)] = cur
	}
	cur.version++
	cur.ct = w.Ciphertext
	return Secret{ID: cur.id, Key: w.Key, Path: w.Path, Version: cur.version}, cur.version == 1, nil
}

func (m *memStore) Get(_ context.Context, _, id uuid.UUID) (*Stored, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.secrets {
		if s.id == id {
			return &Stored{Secret: Secret{ID: s.id, Key: s.key, Path: s.path, Version: s.version, Env: m.envSlug[s.env]}, Ciphertext: s.ct}, nil
		}
	}
	return nil, ErrSecretNotFound
}

func (m *memStore) Lookup(_ context.Context, env uuid.UUID, path, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.secrets[sk(env, path, key)]; s != nil {
		return s.ct, nil
	}
	return nil, ErrSecretNotFound
}

func (m *memStore) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.secrets {
		if s.id == id {
			delete(m.secrets, k)
		}
	}
	return nil
}

func (m *memStore) VersionAt(_ context.Context, env uuid.UUID, path, key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.secrets[sk(env, path, key)]; s != nil {
		return s.version, nil
	}
	return 0, nil
}

func (m *memStore) DeleteAt(_ context.Context, env uuid.UUID, path, key string, version int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.secrets[sk(env, path, key)]
	if s == nil || s.version != version {
		return ErrStaleChange
	}
	delete(m.secrets, sk(env, path, key))
	return nil
}

func (m *memStore) CreateChange(_ context.Context, n NewChange) (uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.changes {
		if c.Status == "pending" && c.EnvID == n.EnvID && c.Path == n.Path && c.Key == n.Key {
			return uuid.Nil, ErrChangeOpen
		}
	}
	id := uuid.New()
	m.changes[id] = &StoredChange{ChangeRequest: ChangeRequest{ID: id, EnvID: n.EnvID, Env: m.envSlug[n.EnvID], Path: n.Path, Key: n.Key, Op: n.Op,
		Comment: n.Comment, BaseVersion: n.BaseVersion, Status: "pending", AuthorID: n.AuthorID, Author: n.AuthorLabel, CreatedAt: time.Now()}, ValueEnc: n.ValueEnc}
	return id, nil
}

func (m *memStore) view(c *StoredChange) StoredChange {
	out := *c
	if s := m.secrets[sk(c.EnvID, c.Path, c.Key)]; s != nil {
		out.CurrentVersion = s.version
	} else {
		out.CurrentVersion = 0
	}
	out.Stale = out.Status == "pending" && out.CurrentVersion != out.BaseVersion
	return out
}

func (m *memStore) ListChanges(_ context.Context, _ uuid.UUID, status string) ([]ChangeRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []ChangeRequest{}
	for _, c := range m.changes {
		if status == "" || c.Status == status {
			out = append(out, m.view(c).ChangeRequest)
		}
	}
	return out, nil
}

func (m *memStore) GetChange(_ context.Context, _, id uuid.UUID) (*StoredChange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.changes[id]
	if !ok {
		return nil, ErrChangeNotFound
	}
	v := m.view(c)
	return &v, nil
}

func (m *memStore) DecideChange(_ context.Context, id uuid.UUID, status string, reviewer *uuid.UUID, label, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.changes[id]
	if c == nil || c.Status != "pending" {
		return ErrChangeNotPending
	}
	c.Status, c.ReviewerID, c.Reviewer, c.Note = status, reviewer, label, note
	return nil
}

func (m *memStore) ReopenChange(_ context.Context, id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.changes[id]; c != nil && c.Status == "applied" {
		c.Status, c.ReviewerID, c.Reviewer = "pending", nil, ""
	}
}

type memKeys struct{ wrapped []byte }

func (k memKeys) WrappedKey(context.Context, uuid.UUID) ([]byte, error) { return k.wrapped, nil }

type world struct {
	svc                    *Service
	store                  *memStore
	project                uuid.UUID
	admin, dev1, dev2, vwr principal.Actor
	events                 []string
	mu                     sync.Mutex
}

func newWorld(t *testing.T) *world {
	t.Helper()
	mk := make([]byte, 32)
	rand.Read(mk)
	local, err := crypto.NewLocal(base64.StdEncoding.EncodeToString(mk))
	if err != nil {
		t.Fatal(err)
	}
	_, wrapped, err := local.NewDEK(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	k := make([]byte, 32)
	rand.Read(k)
	box := pii.New(k, k)

	w := &world{project: uuid.New()}
	w.admin = principal.Actor{UserID: uuid.New(), Email: "admin@x.io"}
	w.dev1 = principal.Actor{UserID: uuid.New(), Email: "dev1@x.io"}
	w.dev2 = principal.Actor{UserID: uuid.New(), Email: "dev2@x.io"}
	w.vwr = principal.Actor{UserID: uuid.New(), Email: "viewer@x.io"}
	dev, prod := access.Env{ID: uuid.New(), Slug: "dev"}, access.Env{ID: uuid.New(), Slug: "prod"}
	acc := &memAccess{org: uuid.New(), envs: []access.Env{dev, prod}, prot: map[uuid.UUID]bool{prod.ID: true},
		roles: map[uuid.UUID]string{w.admin.UserID: "admin", w.dev1.UserID: "developer", w.dev2.UserID: "developer", w.vwr.UserID: "viewer"}}
	w.store = &memStore{envSlug: map[uuid.UUID]string{dev.ID: "dev", prod.ID: "prod"}, secrets: map[string]*memSecret{}, changes: map[uuid.UUID]*StoredChange{}}
	au := audit.New(nil, nil, "")
	au.Subscribe(func(e audit.Event) { w.mu.Lock(); w.events = append(w.events, e.Action); w.mu.Unlock() })
	w.svc = NewService(w.store, access.NewService(acc, box), keystore.NewService(local, memKeys{wrapped}, time.Minute), box, au)
	return w
}

func (w *world) put(t *testing.T, a principal.Actor, env, key, val string) (*Secret, *ChangeRequest, error) {
	t.Helper()
	return w.svc.Put(context.Background(), a, w.project, &PutRequest{Env: env, Path: "/", Key: key, Value: val})
}

func (w *world) value(t *testing.T, env, key string) (string, bool) {
	t.Helper()
	for _, s := range w.store.secrets {
		if w.store.envSlug[s.env] == env && s.key == key {
			st, _ := w.store.Get(context.Background(), w.project, s.id)
			v, err := w.svc.decrypt(context.Background(), w.project, st.Ciphertext)
			if err != nil {
				t.Fatal(err)
			}
			return v, true
		}
	}
	return "", false
}

func (w *world) saw(action string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range w.events {
		if e == action {
			return true
		}
	}
	return false
}

// ---- tests ----

func TestUnprotectedEnvWritesDirectly(t *testing.T) {
	w := newWorld(t)
	sec, cr, err := w.put(t, w.dev1, "dev", "HOST", "db.local")
	if err != nil || sec == nil || cr != nil {
		t.Fatalf("got %v, %v, %v", sec, cr, err)
	}
}

func TestAdminBypassesProtection(t *testing.T) {
	w := newWorld(t)
	sec, cr, err := w.put(t, w.admin, "prod", "HOST", "db.prod")
	if err != nil || sec == nil || cr != nil {
		t.Fatalf("admin write on prod: %v, %v, %v", sec, cr, err)
	}
}

func TestProtectedWriteBecomesRequest(t *testing.T) {
	w := newWorld(t)
	sec, cr, err := w.put(t, w.dev1, "prod", "HOST", "db.prod")
	if err != nil || sec != nil || cr == nil {
		t.Fatalf("got %v, %v, %v", sec, cr, err)
	}
	if cr.Status != "pending" || cr.Op != "set" || cr.BaseVersion != 0 || cr.Author != "dev1@x.io" {
		t.Fatalf("request: %+v", cr)
	}
	if _, ok := w.value(t, "prod", "HOST"); ok {
		t.Fatal("nothing may be written before approval")
	}
	for _, c := range w.store.changes {
		if string(c.ValueEnc) == "db.prod" {
			t.Fatal("proposed value must be stored encrypted")
		}
	}
	if !w.saw("change.request") {
		t.Fatal("request not audited")
	}
	if _, _, err := w.put(t, w.dev2, "prod", "HOST", "other"); !errors.Is(err, ErrChangeOpen) {
		t.Fatalf("second open request for the same key: %v", err)
	}
}

func TestViewerCannotPropose(t *testing.T) {
	w := newWorld(t)
	if _, _, err := w.put(t, w.vwr, "prod", "HOST", "x"); err == nil {
		t.Fatal("viewer must not be able to open a change request")
	}
}

func TestApproveApplies(t *testing.T) {
	w := newWorld(t)
	_, cr, _ := w.put(t, w.dev1, "prod", "HOST", "db.prod")

	out, err := w.svc.Approve(context.Background(), w.dev2, w.project, cr.ID, "lgtm")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "applied" || out.Reviewer != "dev2@x.io" {
		t.Fatalf("after approve: %+v", out)
	}
	if v, ok := w.value(t, "prod", "HOST"); !ok || v != "db.prod" {
		t.Fatalf("value not applied: %q %v", v, ok)
	}
	if !w.saw("change.approve") || !w.saw("secret.create") {
		t.Fatalf("missing audit events: %v", w.events)
	}
	if _, err := w.svc.Approve(context.Background(), w.admin, w.project, cr.ID, ""); !errors.Is(err, ErrChangeNotPending) {
		t.Fatalf("approving twice: %v", err)
	}
}

func TestCannotApproveOwnChange(t *testing.T) {
	w := newWorld(t)
	_, cr, _ := w.put(t, w.dev1, "prod", "HOST", "db.prod")
	if _, err := w.svc.Approve(context.Background(), w.dev1, w.project, cr.ID, ""); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self approval: %v", err)
	}
	if _, err := w.svc.Reject(context.Background(), w.dev1, w.project, cr.ID, ""); !errors.Is(err, ErrSelfReview) {
		t.Fatalf("self reject: %v", err)
	}
}

func TestViewerCannotApprove(t *testing.T) {
	w := newWorld(t)
	_, cr, _ := w.put(t, w.dev1, "prod", "HOST", "db.prod")
	if _, err := w.svc.Approve(context.Background(), w.vwr, w.project, cr.ID, ""); err == nil {
		t.Fatal("viewer approved a change")
	}
}

func TestStaleRequestIsRefused(t *testing.T) {
	w := newWorld(t)
	_, cr, _ := w.put(t, w.dev1, "prod", "HOST", "proposed")
	// An admin edits the secret directly after the request was opened.
	if _, _, err := w.put(t, w.admin, "prod", "HOST", "newer"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Approve(context.Background(), w.dev2, w.project, cr.ID, ""); !errors.Is(err, ErrStaleChange) {
		t.Fatalf("stale approve: %v", err)
	}
	if v, _ := w.value(t, "prod", "HOST"); v != "newer" {
		t.Fatalf("newer value was overwritten: %q", v)
	}
	list, _ := w.svc.Changes(context.Background(), w.dev2, w.project, "pending")
	if len(list) != 1 || !list[0].Stale {
		t.Fatalf("request should be listed as stale: %+v", list)
	}
}

func TestConcurrentApprovalsApplyOnce(t *testing.T) {
	w := newWorld(t)
	_, cr, _ := w.put(t, w.dev1, "prod", "HOST", "db.prod")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for _, a := range []principal.Actor{w.dev2, w.admin, w.dev2, w.admin} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := w.svc.Approve(context.Background(), a, w.project, cr.ID, ""); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("applied %d times, want 1", ok)
	}
	for _, s := range w.store.secrets {
		if s.version != 1 {
			t.Fatalf("secret written %d times", s.version)
		}
	}
}

func TestRejectAndCancel(t *testing.T) {
	w := newWorld(t)
	_, cr, _ := w.put(t, w.dev1, "prod", "A", "1")
	out, err := w.svc.Reject(context.Background(), w.dev2, w.project, cr.ID, "wrong host")
	if err != nil || out.Status != "rejected" || out.Note != "wrong host" {
		t.Fatalf("reject: %+v %v", out, err)
	}
	if _, ok := w.value(t, "prod", "A"); ok {
		t.Fatal("rejected change was applied")
	}

	_, cr2, _ := w.put(t, w.dev1, "prod", "B", "2")
	if _, err := w.svc.CancelChange(context.Background(), w.dev2, w.project, cr2.ID); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("cancel by someone else: %v", err)
	}
	if out, err := w.svc.CancelChange(context.Background(), w.dev1, w.project, cr2.ID); err != nil || out.Status != "cancelled" {
		t.Fatalf("cancel by author: %+v %v", out, err)
	}
}

func TestProtectedDeleteBecomesRequest(t *testing.T) {
	w := newWorld(t)
	sec, _, _ := w.put(t, w.admin, "prod", "OLD", "x")
	cr, err := w.svc.Delete(context.Background(), w.dev1, w.project, sec.ID)
	if err != nil || cr == nil || cr.Op != "delete" || cr.BaseVersion != 1 {
		t.Fatalf("delete request: %+v %v", cr, err)
	}
	if _, ok := w.value(t, "prod", "OLD"); !ok {
		t.Fatal("deleted before approval")
	}
	if _, err := w.svc.Approve(context.Background(), w.dev2, w.project, cr.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.value(t, "prod", "OLD"); ok {
		t.Fatal("approved delete did not delete")
	}
}

func TestBulkAndGeneratedWritesBlockedOnProtected(t *testing.T) {
	w := newWorld(t)
	sec, _, _ := w.put(t, w.admin, "prod", "K", "v")
	ctx := context.Background()
	if _, err := w.svc.Rotate(ctx, w.dev1, w.project, sec.ID, &RotateRequest{}); !errors.Is(err, ErrProtectedEnv) {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := w.svc.Rollback(ctx, w.dev1, w.project, sec.ID, &RollbackRequest{Version: 1}); !errors.Is(err, ErrProtectedEnv) {
		t.Fatalf("rollback: %v", err)
	}
	if _, err := w.svc.Import(ctx, w.dev1, w.project, &ImportRequest{Env: "prod", Content: "A=1"}); !errors.Is(err, ErrProtectedEnv) {
		t.Fatalf("import: %v", err)
	}
}

func TestChangeDetailShowsBothValues(t *testing.T) {
	w := newWorld(t)
	w.put(t, w.admin, "prod", "HOST", "old")
	_, cr, _ := w.put(t, w.dev1, "prod", "HOST", "new")
	d, err := w.svc.Change(context.Background(), w.dev2, w.project, cr.ID)
	if err != nil || d.Proposed == nil || *d.Proposed != "new" || d.Current == nil || *d.Current != "old" {
		t.Fatalf("detail: %+v %v", d, err)
	}
	if !w.saw("change.view") {
		t.Fatal("viewing a proposal must be audited")
	}
}
