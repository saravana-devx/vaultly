package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/crypto"
	"vaultly/internal/keystore"
	"vaultly/internal/pathutil"
	"vaultly/internal/principal"
)

type IntegrationRepository interface {
	List(ctx context.Context, projectID uuid.UUID) ([]Row, error)
	Create(ctx context.Context, n NewIntegration) (uuid.UUID, error)
	SetAutoSync(ctx context.Context, id, projectID uuid.UUID, on bool) error
	Delete(ctx context.Context, id, projectID uuid.UUID) (string, error)
	SyncInfo(ctx context.Context, id uuid.UUID) (*SyncInfo, error)
	MarkSynced(ctx context.Context, id uuid.UUID, status, msg string)
	AutoSyncIDs(ctx context.Context, projectID uuid.UUID, env string) ([]uuid.UUID, error)
}

// SecretSource yields the resolved secrets of an environment path.
type SecretSource interface {
	Snapshot(ctx context.Context, projectID, envID uuid.UUID, envSlug, path string) (map[string]string, error)
}

type Service struct {
	repo      IntegrationRepository
	access    *access.Service
	keys      *keystore.Service
	secrets   SecretSource
	providers map[string]Provider
	audit     *audit.Service

	mu     sync.Mutex
	timers map[uuid.UUID]*time.Timer
}

func NewService(repo IntegrationRepository, access *access.Service, keys *keystore.Service, secrets SecretSource, providers map[string]Provider, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, keys: keys, secrets: secrets, providers: providers, audit: audit, timers: map[uuid.UUID]*time.Timer{}}
}

func (s *Service) encryptConfig(projectID uuid.UUID, dek []byte, v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return crypto.Encrypt(dek, b, []byte(projectID.String()))
}

func (s *Service) decryptConfig(projectID uuid.UUID, dek, enc []byte, v any) error {
	b, err := crypto.Decrypt(dek, enc, []byte(projectID.String()))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Service) List(ctx context.Context, a principal.Actor, projectID uuid.UUID) ([]View, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		var cfg map[string]string
		_ = s.decryptConfig(projectID, dek, r.ConfigEnc, &cfg)
		r.Config = map[string]string{}
		if prov, ok := s.providers[r.Provider]; ok {
			r.Config = prov.Public(cfg)
		}
		out = append(out, r.View)
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, a principal.Actor, projectID uuid.UUID, req *CreateRequest) (*Created, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return nil, err
	}
	prov, ok := s.providers[req.Provider]
	if !ok {
		return nil, ErrUnknownProvider
	}
	if err := prov.Validate(req.Config); err != nil {
		return nil, invalid(err)
	}
	path, ok := pathutil.Norm(req.Path)
	envID, envErr := s.access.EnvID(ctx, projectID, req.Env)
	if !ok || envErr != nil {
		return nil, ErrInvalidEnvPath
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	enc, err := s.encryptConfig(projectID, dek, req.Config)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	id, err := s.repo.Create(ctx, NewIntegration{ProjectID: projectID, EnvID: envID, Path: path, Provider: req.Provider, Name: name, ConfigEnc: enc, AutoSync: req.AutoSync})
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "integration.create", name, map[string]any{"provider": req.Provider, "env": req.Env})
	return &Created{ID: id}, nil
}

func (s *Service) SetAutoSync(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, req *UpdateRequest) error {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return err
	}
	return s.repo.SetAutoSync(ctx, id, projectID, *req.AutoSync)
}

func (s *Service) Delete(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return err
	}
	name, err := s.repo.Delete(ctx, id, projectID)
	if err != nil {
		return err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "integration.delete", name, nil)
	return nil
}

func (s *Service) SyncNow(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) (*SyncResult, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	info, err := s.repo.SyncInfo(ctx, id)
	if err != nil || info.ProjectID != projectID {
		return nil, ErrNotFound
	}
	res, err := s.sync(ctx, info, "user", a.UserID.String(), a.Email)
	if err != nil {
		return nil, upstream(err)
	}
	return &SyncResult{Pushed: res.Pushed, Skipped: res.Skipped}, nil
}

// sync pushes the resolved secrets of an integration's environment path to its provider
// and records the outcome on the integration and in the audit trail.
func (s *Service) sync(ctx context.Context, info *SyncInfo, actorType, actorID, actorLabel string) (res Result, err error) {
	defer func() {
		status, msg := "ok", ""
		if err != nil {
			status, msg = "error", err.Error()
			if len(msg) > 300 {
				msg = msg[:300]
			}
		}
		bg := context.WithoutCancel(ctx)
		s.repo.MarkSynced(bg, info.ID, status, msg)
		md := map[string]any{"provider": info.Provider, "env": info.EnvSlug, "status": status, "pushed": res.Pushed}
		if err != nil {
			md["error"] = msg
		}
		s.audit.Publish(bg, audit.Event{OrgID: info.OrgID, ProjectID: &info.ProjectID, ActorType: actorType, ActorID: actorID, ActorLabel: actorLabel,
			Action: "integration.sync", Resource: info.Name, Metadata: md})
	}()

	prov, ok := s.providers[info.Provider]
	if !ok {
		return res, fmt.Errorf("unknown provider %q", info.Provider)
	}
	dek, err := s.keys.ProjectKey(ctx, info.ProjectID)
	if err != nil {
		return res, err
	}
	var cfg map[string]string
	if err = s.decryptConfig(info.ProjectID, dek, info.ConfigEnc, &cfg); err != nil {
		return res, err
	}
	secrets, err := s.secrets.Snapshot(ctx, info.ProjectID, info.EnvID, info.EnvSlug, info.Path)
	if err != nil {
		return res, err
	}
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return prov.Push(pctx, cfg, secrets)
}

var syncTriggers = map[string]bool{"secret.create": true, "secret.update": true, "secret.rotate": true, "secret.rollback": true, "secret.import": true}

// AutoSync is an audit subscriber: secret writes schedule a debounced push for every
// auto-sync integration in the project. Deletions are not propagated.
func (s *Service) AutoSync(ev audit.Event) {
	if ev.ProjectID == nil || !syncTriggers[ev.Action] {
		return
	}
	env, _ := ev.Metadata["env"].(string)
	ids, err := s.repo.AutoSyncIDs(context.Background(), *ev.ProjectID, env)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.schedule(id)
	}
}

func (s *Service) schedule(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.timers[id]; ok {
		t.Stop()
	}
	s.timers[id] = time.AfterFunc(3*time.Second, func() {
		s.mu.Lock()
		delete(s.timers, id)
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		info, err := s.repo.SyncInfo(ctx, id)
		if err != nil {
			return
		}
		_, _ = s.sync(ctx, info, "system", "auto-sync", "system:auto-sync")
	})
}
