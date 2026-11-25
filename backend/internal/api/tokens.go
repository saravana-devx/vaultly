package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	tokenPrefix    = "vst_"
	personalPrefix = "vpt_"
)

func newToken(prefix string) (raw string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b)
	raw = prefix + base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	return raw, h[:]
}

// normCIDRs accepts single IPs or CIDR blocks and returns canonical CIDRs.
func normCIDRs(in []string) ([]string, bool) {
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, false
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, false
		}
		out = append(out, n.String())
	}
	return out, len(out) <= 20
}

func ipAllowed(cidrs []string, ip string) bool {
	if len(cidrs) == 0 {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}

func (s *Server) listTokens(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT t.id,t.name,t.token_prefix,e.slug,t.path_prefix,t.can_write,t.allowed_cidrs,t.expires_at,t.last_used_at,t.created_by,t.created_at
		FROM service_tokens t JOIN environments e ON e.id=t.env_id WHERE t.project_id=$1 ORDER BY t.created_at DESC`, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var name, prefix, env, path, by string
		var write bool
		var cidrs []string
		var exp, used *time.Time
		var at time.Time
		if rows.Scan(&id, &name, &prefix, &env, &path, &write, &cidrs, &exp, &used, &by, &at) == nil {
			out = append(out, gin.H{"id": id, "name": name, "prefix": prefix, "env": env, "path": path, "can_write": write,
				"allowed_cidrs": cidrs, "expires_at": exp, "last_used_at": used, "created_by": s.openLabel(by), "created_at": at})
		}
	}
	c.JSON(200, out)
}

func (s *Server) createToken(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var in struct {
		Name         string
		Env          string
		Path         string
		CanWrite     bool     `json:"can_write"`
		ExpireDays   int      `json:"expire_days"`
		AllowedCIDRs []string `json:"allowed_cidrs"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "name is required")
		return
	}
	path, pok := normPath(in.Path)
	eid, err := s.envID(c, p.ID, in.Env)
	if !pok || err != nil {
		fail(c, 400, "invalid environment or path")
		return
	}
	cidrs, cok := normCIDRs(in.AllowedCIDRs)
	if !cok {
		fail(c, 400, "allowed IPs must be valid addresses or CIDR ranges (max 20)")
		return
	}
	raw, hash := newToken(tokenPrefix)
	var exp *time.Time
	if in.ExpireDays > 0 {
		t := time.Now().Add(time.Duration(in.ExpireDays) * 24 * time.Hour)
		exp = &t
	}
	var id uuid.UUID
	err = s.db.QueryRow(c, `INSERT INTO service_tokens (project_id,env_id,name,token_prefix,token_hash,path_prefix,can_write,allowed_cidrs,expires_at,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		p.ID, eid, strings.TrimSpace(in.Name), raw[:10], hash, path, in.CanWrite, cidrs, exp, s.sealLabel(actorLabel(c))).Scan(&id)
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "token.create", in.Name, map[string]any{"env": in.Env, "path": path, "can_write": in.CanWrite, "ip_restricted": len(cidrs) > 0})
	c.JSON(201, gin.H{"id": id, "token": raw, "name": in.Name})
}

func (s *Server) deleteToken(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	tid, err := uuid.Parse(c.Param("tid"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	var name string
	if s.db.QueryRow(c, `DELETE FROM service_tokens WHERE id=$1 AND project_id=$2 RETURNING name`, tid, p.ID).Scan(&name) != nil {
		fail(c, 404, "not found")
		return
	}
	s.record(c, p.OrgID, &p.ID, "token.revoke", name, nil)
	c.Status(204)
}

// ---- personal access tokens (act as the user, with the user's roles) ----

func (s *Server) listPersonalTokens(c *gin.Context) {
	rows, err := s.db.Query(c, `SELECT id,name,token_prefix,expires_at,last_used_at,created_at FROM personal_tokens WHERE user_id=$1 ORDER BY created_at DESC`, me(c).UserID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var name, prefix string
		var exp, used *time.Time
		var at time.Time
		if rows.Scan(&id, &name, &prefix, &exp, &used, &at) == nil {
			out = append(out, gin.H{"id": id, "name": name, "prefix": prefix, "expires_at": exp, "last_used_at": used, "created_at": at})
		}
	}
	c.JSON(200, out)
}

func (s *Server) createPersonalToken(c *gin.Context) {
	if me(c).ViaPAT {
		fail(c, 403, "sign in with your password to manage personal tokens")
		return
	}
	var in struct {
		Name       string
		ExpireDays int `json:"expire_days"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "name is required")
		return
	}
	raw, hash := newToken(personalPrefix)
	var exp *time.Time
	if in.ExpireDays > 0 {
		t := time.Now().Add(time.Duration(in.ExpireDays) * 24 * time.Hour)
		exp = &t
	}
	var id uuid.UUID
	if err := s.db.QueryRow(c, `INSERT INTO personal_tokens (user_id,name,token_prefix,token_hash,expires_at) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		me(c).UserID, strings.TrimSpace(in.Name), raw[:10], hash, exp).Scan(&id); err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id, "token": raw, "name": in.Name})
}

func (s *Server) deletePersonalToken(c *gin.Context) {
	if me(c).ViaPAT {
		fail(c, 403, "sign in with your password to manage personal tokens")
		return
	}
	tid, err := uuid.Parse(c.Param("tid"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	s.db.Exec(c, `DELETE FROM personal_tokens WHERE id=$1 AND user_id=$2`, tid, me(c).UserID)
	c.Status(204)
}

// ---- machine API (service tokens) ----

func (s *Server) tokenScope(c *gin.Context, requested string) (string, bool) {
	t := me(c).Token
	if requested == "" {
		requested = t.PathPrefix
	}
	path, ok := normPath(requested)
	if !ok || !within(t.PathPrefix, path) {
		fail(c, 403, "path is outside this token's scope ("+t.PathPrefix+")")
		return "", false
	}
	return path, true
}

func (s *Server) tokenAllow(t *tokenInfo) func(env, path string) bool {
	return func(env, path string) bool { return env == t.EnvSlug && within(t.PathPrefix, path) }
}

func (s *Server) tokenGetSecrets(c *gin.Context) {
	t := me(c).Token
	path, ok := s.tokenScope(c, c.Query("path"))
	if !ok {
		return
	}
	pairs, err := s.collect(c, t.ProjectID, t.EnvID, t.EnvSlug, path, fetchOpts{Recursive: c.Query("recursive") == "true", Resolve: c.Query("resolve") != "false", Allow: s.tokenAllow(t)})
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, t.OrgID, &t.ProjectID, "secret.fetch", path, map[string]any{"count": len(pairs), "env": t.EnvSlug})
	respondSecrets(c, pairs, c.DefaultQuery("format", "json"))
}

func (s *Server) tokenPutSecret(c *gin.Context) {
	t := me(c).Token
	if !t.CanWrite {
		fail(c, 403, "this token is read-only")
		return
	}
	var in struct{ Path, Key, Value, Comment string }
	if c.ShouldBindJSON(&in) != nil || !keyRe.MatchString(in.Key) || len(in.Value) > maxValueLen {
		fail(c, 400, "invalid request")
		return
	}
	path, ok := s.tokenScope(c, in.Path)
	if !ok {
		return
	}
	dek, err := s.dek(c, t.ProjectID)
	if err != nil {
		internalErr(c, err)
		return
	}
	r, created, err := s.writeSecret(c, writeInput{ProjectID: t.ProjectID, EnvID: t.EnvID, DEK: dek, Path: path, Key: in.Key, Value: in.Value, Comment: in.Comment, Actor: actorLabel(c)})
	if err != nil {
		internalErr(c, err)
		return
	}
	act := "secret.update"
	if created {
		act = "secret.create"
	}
	s.record(c, t.OrgID, &t.ProjectID, act, join(path, in.Key), map[string]any{"version": r.Version, "env": t.EnvSlug})
	c.JSON(200, gin.H{"key": r.Key, "path": r.Path, "version": r.Version})
}
