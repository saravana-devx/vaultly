package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/saravana/vaultly/internal/audit"
	"github.com/saravana/vaultly/internal/crypto"
	"github.com/saravana/vaultly/internal/integrations"
)

type syncState struct {
	mu     sync.Mutex
	timers map[uuid.UUID]*time.Timer
}

func (s *Server) encryptConfig(projectID uuid.UUID, dek []byte, v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return crypto.Encrypt(dek, b, []byte(projectID.String()))
}

func (s *Server) decryptConfig(projectID uuid.UUID, dek, enc []byte, v any) error {
	b, err := crypto.Decrypt(dek, enc, []byte(projectID.String()))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Server) listIntegrations(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	rows, err := s.db.Query(c, `SELECT i.id,i.name,i.provider,e.slug,i.path,i.auto_sync,i.config_enc,i.last_synced_at,i.last_status,i.last_error
		FROM integrations i JOIN environments e ON e.id=i.env_id WHERE i.project_id=$1 ORDER BY i.created_at DESC`, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var name, provider, env, path, status, lastErr string
		var auto bool
		var enc []byte
		var at *time.Time
		if rows.Scan(&id, &name, &provider, &env, &path, &auto, &enc, &at, &status, &lastErr) != nil {
			continue
		}
		var cfg map[string]string
		_ = s.decryptConfig(p.ID, dek, enc, &cfg)
		pub := map[string]string{}
		if prov, ok := s.providers[provider]; ok {
			pub = prov.Public(cfg)
		}
		out = append(out, gin.H{"id": id, "name": name, "provider": provider, "env": env, "path": path, "auto_sync": auto,
			"config": pub, "last_synced_at": at, "last_status": status, "last_error": lastErr})
	}
	c.JSON(200, out)
}

func (s *Server) createIntegration(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var in struct {
		Provider string
		Name     string
		Env      string
		Path     string
		Config   map[string]string
		AutoSync bool `json:"auto_sync"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "provider, name, environment and config are required")
		return
	}
	prov, ok := s.providers[in.Provider]
	if !ok {
		fail(c, 400, "provider must be github, vercel or railway")
		return
	}
	if err := prov.Validate(in.Config); err != nil {
		fail(c, 400, err.Error())
		return
	}
	path, pok := normPath(in.Path)
	eid, err := s.envID(c, p.ID, in.Env)
	if !pok || err != nil {
		fail(c, 400, "invalid environment or path")
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	enc, err := s.encryptConfig(p.ID, dek, in.Config)
	if err != nil {
		internalErr(c, err)
		return
	}
	var id uuid.UUID
	if err := s.db.QueryRow(c, `INSERT INTO integrations (project_id,env_id,path,provider,name,config_enc,auto_sync) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		p.ID, eid, path, in.Provider, strings.TrimSpace(in.Name), enc, in.AutoSync).Scan(&id); err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "integration.create", in.Name, map[string]any{"provider": in.Provider, "env": in.Env})
	c.JSON(201, gin.H{"id": id})
}

func (s *Server) updateIntegration(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var in struct {
		AutoSync *bool `json:"auto_sync"`
	}
	if c.ShouldBindJSON(&in) != nil || in.AutoSync == nil {
		fail(c, 400, "invalid request")
		return
	}
	tag, err := s.db.Exec(c, `UPDATE integrations SET auto_sync=$3 WHERE id=$1 AND project_id=$2`, c.Param("iid"), p.ID, *in.AutoSync)
	if err != nil || tag.RowsAffected() == 0 {
		fail(c, 404, "not found")
		return
	}
	c.Status(204)
}

func (s *Server) deleteIntegration(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var name string
	if s.db.QueryRow(c, `DELETE FROM integrations WHERE id=$1 AND project_id=$2 RETURNING name`, c.Param("iid"), p.ID).Scan(&name) != nil {
		fail(c, 404, "not found")
		return
	}
	s.record(c, p.OrgID, &p.ID, "integration.delete", name, nil)
	c.Status(204)
}

func (s *Server) syncNow(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	iid, err := uuid.Parse(c.Param("iid"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	var owner uuid.UUID
	if s.db.QueryRow(c, `SELECT project_id FROM integrations WHERE id=$1`, iid).Scan(&owner) != nil || owner != p.ID {
		fail(c, 404, "not found")
		return
	}
	res, err := s.syncIntegration(c, iid, "user", me(c).UserID.String(), me(c).Email)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"pushed": res.Pushed, "skipped": res.Skipped})
}

// syncIntegration pushes the resolved secrets of an integration's environment
// and path to the external provider and records the outcome.
func (s *Server) syncIntegration(ctx context.Context, id uuid.UUID, actorType, actorID, actorLabel string) (res integrations.Result, err error) {
	var projectID, orgID, envID uuid.UUID
	var name, provider, envSlug, path string
	var enc []byte
	if err = s.db.QueryRow(ctx, `SELECT i.project_id, p.org_id, i.env_id, i.name, i.provider, e.slug, i.path, i.config_enc
		FROM integrations i JOIN projects p ON p.id=i.project_id JOIN environments e ON e.id=i.env_id WHERE i.id=$1`, id).
		Scan(&projectID, &orgID, &envID, &name, &provider, &envSlug, &path, &enc); err != nil {
		return res, err
	}
	finish := func(err error) {
		status, msg := "ok", ""
		if err != nil {
			status, msg = "error", err.Error()
			if len(msg) > 300 {
				msg = msg[:300]
			}
		}
		s.db.Exec(context.WithoutCancel(ctx), `UPDATE integrations SET last_synced_at=now(), last_status=$2, last_error=$3 WHERE id=$1`, id, status, msg)
		md := map[string]any{"provider": provider, "env": envSlug, "status": status, "pushed": res.Pushed}
		if err != nil {
			md["error"] = msg
		}
		s.audit.Publish(context.WithoutCancel(ctx), audit.Event{OrgID: orgID, ProjectID: &projectID, ActorType: actorType, ActorID: actorID, ActorLabel: actorLabel,
			Action: "integration.sync", Resource: name, Metadata: md})
	}
	defer func() { finish(err) }()

	prov, ok := s.providers[provider]
	if !ok {
		return res, fmt.Errorf("unknown provider %q", provider)
	}
	dek, err := s.dek(ctx, projectID)
	if err != nil {
		return res, err
	}
	var cfg map[string]string
	if err = s.decryptConfig(projectID, dek, enc, &cfg); err != nil {
		return res, err
	}
	pairs, err := s.collect(ctx, projectID, envID, envSlug, path, fetchOpts{Recursive: true, Resolve: true})
	if err != nil {
		return res, err
	}
	secrets := make(map[string]string, len(pairs))
	for _, kv := range pairs {
		secrets[kv[0]] = kv[1]
	}
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return prov.Push(pctx, cfg, secrets)
}

var syncTriggers = map[string]bool{"secret.create": true, "secret.update": true, "secret.rotate": true, "secret.rollback": true, "secret.import": true}

// autoSync is an audit subscriber: secret writes schedule a debounced push for
// every auto-sync integration in the project. Deletions are not propagated.
func (s *Server) autoSync(ev audit.Event) {
	if ev.ProjectID == nil || !syncTriggers[ev.Action] {
		return
	}
	env, _ := ev.Metadata["env"].(string)
	rows, err := s.db.Query(context.Background(), `SELECT i.id FROM integrations i JOIN environments e ON e.id=i.env_id
		WHERE i.project_id=$1 AND i.auto_sync AND ($2 = '' OR e.slug=$2)`, *ev.ProjectID, env)
	if err != nil {
		return
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		s.sync.mu.Lock()
		if t, ok := s.sync.timers[id]; ok {
			t.Stop()
		}
		id := id
		s.sync.timers[id] = time.AfterFunc(3*time.Second, func() {
			s.sync.mu.Lock()
			delete(s.sync.timers, id)
			s.sync.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s.syncIntegration(ctx, id, "system", "auto-sync", "system:auto-sync")
		})
		s.sync.mu.Unlock()
	}
}
