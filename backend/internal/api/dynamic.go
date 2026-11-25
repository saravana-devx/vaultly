package api

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/saravana/vaultly/internal/dynamic"
)

const lowerAlnum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomLower(n int) string {
	b := make([]byte, n)
	s, _ := randomString(n * 2)
	for i := range b {
		b[i] = lowerAlnum[int(s[i])%len(lowerAlnum)]
	}
	return string(b)
}

type lease struct {
	ID        uuid.UUID `json:"lease_id"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URL       string    `json:"connection_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func hostInfo(adminURL string) gin.H {
	u, err := url.Parse(adminURL)
	if err != nil {
		return gin.H{}
	}
	return gin.H{"host": u.Host, "database": strings.TrimPrefix(u.Path, "/"), "admin_user": u.User.Username()}
}

func (s *Server) listDynamic(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	rows, err := s.db.Query(c, `SELECT d.id,d.name,d.provider,e.slug,d.config_enc,d.default_ttl_s,d.max_ttl_s,
		(SELECT count(*) FROM dynamic_leases l WHERE l.config_id=d.id AND l.revoked_at IS NULL AND l.expires_at > now())
		FROM dynamic_configs d JOIN environments e ON e.id=d.env_id WHERE d.project_id=$1 ORDER BY d.created_at DESC`, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var name, provider, env string
		var enc []byte
		var def, max, active int
		if rows.Scan(&id, &name, &provider, &env, &enc, &def, &max, &active) != nil {
			continue
		}
		var cfg dynamic.Config
		_ = s.decryptConfig(p.ID, dek, enc, &cfg)
		out = append(out, gin.H{"id": id, "name": name, "provider": provider, "env": env, "target": hostInfo(cfg.AdminURL), "grants": cfg.Grants,
			"default_ttl_s": def, "max_ttl_s": max, "active_leases": active})
	}
	c.JSON(200, out)
}

func (s *Server) createDynamic(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var in struct {
		Name       string
		Env        string
		AdminURL   string   `json:"admin_url"`
		Grants     []string `json:"grants"`
		DefaultTTL int      `json:"default_ttl_s"`
		MaxTTL     int      `json:"max_ttl_s"`
	}
	if c.ShouldBindJSON(&in) != nil || slugify(in.Name) == "" {
		fail(c, 400, "name, environment and admin_url are required")
		return
	}
	if in.DefaultTTL <= 0 {
		in.DefaultTTL = 3600
	}
	if in.MaxTTL <= 0 {
		in.MaxTTL = 86400
	}
	if in.DefaultTTL < 60 || in.MaxTTL < in.DefaultTTL || in.MaxTTL > 30*86400 {
		fail(c, 400, "TTLs must be at least 60s, default ≤ max, max ≤ 30 days")
		return
	}
	eid, err := s.envID(c, p.ID, in.Env)
	if err != nil {
		fail(c, 400, "environment not found")
		return
	}
	cfg := dynamic.Config{AdminURL: strings.TrimSpace(in.AdminURL), Grants: in.Grants}
	vctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()
	if err := s.dyn.Validate(vctx, cfg); err != nil {
		fail(c, 400, err.Error())
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	enc, err := s.encryptConfig(p.ID, dek, cfg)
	if err != nil {
		internalErr(c, err)
		return
	}
	var id uuid.UUID
	if err := s.db.QueryRow(c, `INSERT INTO dynamic_configs (project_id,env_id,name,config_enc,default_ttl_s,max_ttl_s) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		p.ID, eid, slugify(in.Name), enc, in.DefaultTTL, in.MaxTTL).Scan(&id); err != nil {
		if isUnique(err) {
			fail(c, 409, "a dynamic secret with that name already exists in this environment")
			return
		}
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "dynamic.create", in.Name, map[string]any{"env": in.Env})
	c.JSON(201, gin.H{"id": id})
}

func (s *Server) deleteDynamic(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var name string
	if s.db.QueryRow(c, `SELECT name FROM dynamic_configs WHERE id=$1 AND project_id=$2`, c.Param("did"), p.ID).Scan(&name) != nil {
		fail(c, 404, "not found")
		return
	}
	s.revokeAll(c, c.Param("did"))
	s.db.Exec(c, `DELETE FROM dynamic_configs WHERE id=$1`, c.Param("did"))
	s.record(c, p.OrgID, &p.ID, "dynamic.delete", name, nil)
	c.Status(204)
}

// issueLease creates a short-lived credential for the dynamic config.
func (s *Server) issueLease(ctx context.Context, configID, projectID uuid.UUID, ttlSeconds int, createdBy string) (*lease, string, string, error) {
	var name, env string
	var enc []byte
	var def, max int
	if err := s.db.QueryRow(ctx, `SELECT d.name, e.slug, d.config_enc, d.default_ttl_s, d.max_ttl_s FROM dynamic_configs d JOIN environments e ON e.id=d.env_id
		WHERE d.id=$1 AND d.project_id=$2`, configID, projectID).Scan(&name, &env, &enc, &def, &max); err != nil {
		return nil, "", "", err
	}
	if ttlSeconds <= 0 {
		ttlSeconds = def
	}
	ttlSeconds = min(max, ttlSeconds)
	ttlSeconds = maxInt(60, ttlSeconds)
	dek, err := s.dek(ctx, projectID)
	if err != nil {
		return nil, "", "", err
	}
	var cfg dynamic.Config
	if err := s.decryptConfig(projectID, dek, enc, &cfg); err != nil {
		return nil, "", "", err
	}
	username := "v_" + randomLower(16)
	password, err := randomString(32)
	if err != nil {
		return nil, "", "", err
	}
	exp := time.Now().Add(time.Duration(ttlSeconds) * time.Second).UTC()
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.dyn.Create(cctx, cfg, username, password, exp); err != nil {
		return nil, "", "", err
	}
	connURL, _ := s.dyn.ConnectionURL(cfg, username, password)
	l := &lease{Username: username, Password: password, URL: connURL, ExpiresAt: exp}
	if err := s.db.QueryRow(ctx, `INSERT INTO dynamic_leases (config_id,username,expires_at,created_by) VALUES ($1,$2,$3,$4) RETURNING id`,
		configID, username, exp, s.sealLabel(createdBy)).Scan(&l.ID); err != nil {
		s.dyn.Revoke(ctx, cfg, username)
		return nil, "", "", err
	}
	return l, name, env, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Server) createLease(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	cid, err := uuid.Parse(c.Param("did"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	var env string
	if s.db.QueryRow(c, `SELECT e.slug FROM dynamic_configs d JOIN environments e ON e.id=d.env_id WHERE d.id=$1 AND d.project_id=$2`, cid, p.ID).Scan(&env) != nil {
		fail(c, 404, "not found")
		return
	}
	if !s.needEnv(c, p, env, "developer") {
		return
	}
	var in struct {
		TTL int `json:"ttl_seconds"`
	}
	_ = c.ShouldBindJSON(&in)
	l, name, _, err := s.issueLease(c, cid, p.ID, in.TTL, me(c).Email)
	if err != nil {
		c.JSON(502, gin.H{"error": "could not create credentials: " + err.Error()})
		return
	}
	s.record(c, p.OrgID, &p.ID, "dynamic.lease", name, map[string]any{"env": env, "username": l.Username, "expires_at": l.ExpiresAt})
	c.JSON(201, l)
}

func (s *Server) listLeases(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT l.id,l.username,l.expires_at,l.revoked_at,l.created_by,l.created_at,l.revoke_error FROM dynamic_leases l
		JOIN dynamic_configs d ON d.id=l.config_id WHERE d.id=$1 AND d.project_id=$2 ORDER BY l.created_at DESC LIMIT 30`, c.Param("did"), p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var user, by, rerr string
		var exp, at time.Time
		var rev *time.Time
		if rows.Scan(&id, &user, &exp, &rev, &by, &at, &rerr) == nil {
			state := "active"
			switch {
			case rev != nil:
				state = "revoked"
			case exp.Before(time.Now()):
				state = "expired"
			}
			out = append(out, gin.H{"id": id, "username": user, "expires_at": exp, "revoked_at": rev, "created_by": s.openLabel(by), "created_at": at, "state": state, "revoke_error": rerr})
		}
	}
	c.JSON(200, out)
}

func (s *Server) revokeLease(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	if err := s.revokeOne(c, c.Param("lid"), p.ID); err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	s.record(c, p.OrgID, &p.ID, "dynamic.revoke", c.Param("lid"), nil)
	c.Status(204)
}

// revokeOne drops one lease's credentials. projectID uuid.Nil skips the ownership check (janitor).
func (s *Server) revokeOne(ctx context.Context, leaseID string, projectID uuid.UUID) error {
	var username string
	var enc []byte
	var pid uuid.UUID
	if err := s.db.QueryRow(ctx, `SELECT l.username, d.config_enc, d.project_id FROM dynamic_leases l JOIN dynamic_configs d ON d.id=l.config_id
		WHERE l.id=$1 AND l.revoked_at IS NULL`, leaseID).Scan(&username, &enc, &pid); err != nil {
		return nil
	}
	if projectID != uuid.Nil && pid != projectID {
		return nil
	}
	dek, err := s.dek(ctx, pid)
	if err != nil {
		return err
	}
	var cfg dynamic.Config
	if err := s.decryptConfig(pid, dek, enc, &cfg); err != nil {
		return err
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.dyn.Revoke(rctx, cfg, username); err != nil {
		s.db.Exec(ctx, `UPDATE dynamic_leases SET revoke_error=$2 WHERE id=$1`, leaseID, truncate(err.Error(), 300))
		return err
	}
	s.db.Exec(ctx, `UPDATE dynamic_leases SET revoked_at=now(), revoke_error='' WHERE id=$1`, leaseID)
	return nil
}

func (s *Server) revokeAll(ctx context.Context, configID string) {
	rows, err := s.db.Query(ctx, `SELECT id FROM dynamic_leases WHERE config_id=$1 AND revoked_at IS NULL`, configID)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id.String())
		}
	}
	rows.Close()
	for _, id := range ids {
		s.revokeOne(ctx, id, uuid.Nil)
	}
}

// revokeLeasesWhere revokes every active lease whose config matches the condition,
// so deleting a project or environment never leaves live database roles behind.
func (s *Server) revokeLeasesWhere(ctx context.Context, cond string, arg uuid.UUID) {
	rows, err := s.db.Query(ctx, `SELECT l.id FROM dynamic_leases l JOIN dynamic_configs d ON d.id=l.config_id WHERE l.revoked_at IS NULL AND `+cond, arg)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id.String())
		}
	}
	rows.Close()
	for _, id := range ids {
		s.revokeOne(ctx, id, uuid.Nil)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// tokenLease lets a service token request credentials for a dynamic secret in its own environment.
func (s *Server) tokenLease(c *gin.Context) {
	t := me(c).Token
	var cid uuid.UUID
	if s.db.QueryRow(c, `SELECT id FROM dynamic_configs WHERE project_id=$1 AND env_id=$2 AND name=$3`, t.ProjectID, t.EnvID, c.Param("name")).Scan(&cid) != nil {
		fail(c, 404, "dynamic secret not found in this token's environment")
		return
	}
	var in struct {
		TTL int `json:"ttl_seconds"`
	}
	_ = c.ShouldBindJSON(&in)
	l, name, _, err := s.issueLease(c, cid, t.ProjectID, in.TTL, actorLabel(c))
	if err != nil {
		c.JSON(502, gin.H{"error": "could not create credentials"})
		return
	}
	s.record(c, t.OrgID, &t.ProjectID, "dynamic.lease", name, map[string]any{"env": t.EnvSlug, "username": l.Username, "expires_at": l.ExpiresAt})
	c.JSON(201, l)
}
