package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/saravana/vaultly/internal/crypto"
)

var (
	keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
	tagRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

const maxValueLen = 64 * 1024

func normPath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/", true
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			continue
		}
		if seg == "." || seg == ".." || len(seg) > 64 {
			return "", false
		}
		parts = append(parts, seg)
	}
	return "/" + strings.Join(parts, "/"), true
}

func join(path, key string) string {
	if path == "/" {
		return "/" + key
	}
	return path + "/" + key
}

func within(prefix, path string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

func normTags(in []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")))
		if t == "" || seen[t] {
			continue
		}
		if !tagRe.MatchString(t) {
			return nil, false
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, len(out) <= 10
}

type secretRow struct {
	ID        uuid.UUID  `json:"id"`
	Key       string     `json:"key"`
	Path      string     `json:"path"`
	Version   int        `json:"version"`
	Comment   string     `json:"comment"`
	Tags      []string   `json:"tags"`
	ExpiresAt *time.Time `json:"expires_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	UpdatedBy string     `json:"updated_by"`
	Value     *string    `json:"value,omitempty"`
	Resolved  *string    `json:"resolved,omitempty"`
	Env       string     `json:"env,omitempty"`
}

const secretCols = `s.id, s.key, s.path, s.version, s.comment, s.tags, s.expires_at, s.updated_at, s.updated_by`

func (s *Server) scanSecret(row pgx.Row, extra ...any) (*secretRow, error) {
	r := &secretRow{}
	dst := append([]any{&r.ID, &r.Key, &r.Path, &r.Version, &r.Comment, &r.Tags, &r.ExpiresAt, &r.UpdatedAt, &r.UpdatedBy}, extra...)
	err := row.Scan(dst...)
	r.UpdatedBy = s.openLabel(r.UpdatedBy)
	return r, err
}

type writeInput struct {
	ProjectID, EnvID uuid.UUID
	DEK              []byte
	Path, Key, Value string
	Comment, Actor   string
	Action           string
	Tags             *[]string
	Expires          *time.Time
	ClearExpiry      bool
}

// writeSecret creates or updates a secret and records a version row. Tags and
// expiry carry over from the existing secret unless the caller sets them.
func (s *Server) writeSecret(ctx context.Context, in writeInput) (*secretRow, bool, error) {
	ct, err := crypto.Encrypt(in.DEK, []byte(in.Value), []byte(in.ProjectID.String()))
	if err != nil {
		return nil, false, err
	}
	r := &secretRow{Key: in.Key, Path: in.Path}
	actor := s.sealLabel(in.Actor)
	created := false
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var id uuid.UUID
		var ver int
		var tags []string
		var exp *time.Time
		err := tx.QueryRow(ctx, `SELECT id, version, tags, expires_at FROM secrets WHERE env_id=$1 AND path=$2 AND key=$3 FOR UPDATE`,
			in.EnvID, in.Path, in.Key).Scan(&id, &ver, &tags, &exp)
		action := in.Action
		if in.Tags != nil {
			tags = *in.Tags
		}
		switch {
		case in.ClearExpiry:
			exp = nil
		case in.Expires != nil:
			exp = in.Expires
		}
		if tags == nil {
			tags = []string{}
		}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created = true
			if action == "" {
				action = "create"
			}
			ver = 1
			err = tx.QueryRow(ctx, `INSERT INTO secrets (project_id,env_id,path,key,encrypted_value,version,comment,tags,expires_at,updated_by)
				VALUES ($1,$2,$3,$4,$5,1,$6,$7,$8,$9) RETURNING id, updated_at`,
				in.ProjectID, in.EnvID, in.Path, in.Key, ct, in.Comment, tags, exp, actor).Scan(&id, &r.UpdatedAt)
		case err != nil:
			return err
		default:
			ver++
			if action == "" {
				action = "update"
			}
			err = tx.QueryRow(ctx, `UPDATE secrets SET encrypted_value=$2, version=$3, comment=$4, tags=$5, expires_at=$6, expiry_notified=false,
				updated_by=$7, updated_at=now() WHERE id=$1 RETURNING updated_at`, id, ct, ver, in.Comment, tags, exp, actor).Scan(&r.UpdatedAt)
		}
		if err != nil {
			return err
		}
		r.ID, r.Version, r.Comment, r.UpdatedBy, r.Tags, r.ExpiresAt = id, ver, in.Comment, in.Actor, tags, exp
		_, err = tx.Exec(ctx, `INSERT INTO secret_versions (secret_id,version,encrypted_value,action,actor) VALUES ($1,$2,$3,$4,$5)`,
			id, ver, ct, action, actor)
		return err
	})
	return r, created, err
}

func (s *Server) decrypt(dek []byte, projectID uuid.UUID, ct []byte) (string, error) {
	pt, err := crypto.Decrypt(dek, ct, []byte(projectID.String()))
	return string(pt), err
}

// ---- secret references: ${KEY}, ${/path/KEY}, ${env:/path/KEY} ----

var refRe = regexp.MustCompile(`\$\{([^${}]+)\}`)

type resolver struct {
	s         *Server
	ctx       context.Context
	projectID uuid.UUID
	dek       []byte
	allow     func(env, path string) bool
	envIDs    map[string]uuid.UUID
}

func (s *Server) newResolver(ctx context.Context, projectID uuid.UUID, dek []byte, allow func(env, path string) bool) *resolver {
	return &resolver{s: s, ctx: ctx, projectID: projectID, dek: dek, allow: allow, envIDs: map[string]uuid.UUID{}}
}

func (r *resolver) envID(slug string) (uuid.UUID, bool) {
	if id, ok := r.envIDs[slug]; ok {
		return id, true
	}
	id, err := r.s.envID(r.ctx, r.projectID, slug)
	if err != nil {
		return uuid.Nil, false
	}
	r.envIDs[slug] = id
	return id, true
}

func (r *resolver) expand(val, env, path string, stack map[string]bool, depth int) string {
	if depth > 8 || !strings.Contains(val, "${") {
		return val
	}
	return refRe.ReplaceAllStringFunc(val, func(m string) string {
		inner := m[2 : len(m)-1]
		tenv := env
		if i := strings.Index(inner, ":"); i > 0 {
			tenv, inner = inner[:i], inner[i+1:]
		}
		tpath, key, absolute := path, inner, strings.HasPrefix(inner, "/")
		if absolute {
			i := strings.LastIndex(inner, "/")
			tpath, key = inner[:i], inner[i+1:]
			if tpath == "" {
				tpath = "/"
			}
		}
		candidates := []string{tpath}
		if !absolute && tpath != "/" {
			candidates = append(candidates, "/")
		}
		for _, p := range candidates {
			if v, ok := r.lookup(tenv, p, key, stack, depth); ok {
				return v
			}
		}
		return m
	})
}

func (r *resolver) lookup(env, path, key string, stack map[string]bool, depth int) (string, bool) {
	id := env + "|" + path + "|" + key
	if stack[id] || (r.allow != nil && !r.allow(env, path)) {
		return "", false
	}
	eid, ok := r.envID(env)
	if !ok {
		return "", false
	}
	var ct []byte
	if r.s.db.QueryRow(r.ctx, `SELECT encrypted_value FROM secrets WHERE env_id=$1 AND path=$2 AND key=$3 AND (expires_at IS NULL OR expires_at > now())`,
		eid, path, key).Scan(&ct) != nil {
		return "", false
	}
	v, err := r.s.decrypt(r.dek, r.projectID, ct)
	if err != nil {
		return "", false
	}
	stack[id] = true
	defer delete(stack, id)
	return r.expand(v, env, path, stack, depth+1), true
}

// ---- user-facing handlers ----

func (s *Server) listSecrets(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	slug := c.Query("env")
	if !s.needEnv(c, p, slug, "viewer") {
		return
	}
	path, ok := normPath(c.Query("path"))
	if !ok {
		fail(c, 400, "invalid path")
		return
	}
	eid, err := s.envID(c, p.ID, slug)
	if err != nil {
		fail(c, 404, "environment not found")
		return
	}
	args := []any{eid, path, strings.TrimRight(path, "/") + "/%"}
	q := `SELECT ` + secretCols + ` FROM secrets s WHERE s.env_id=$1 AND (s.path=$2 OR s.path LIKE $3)`
	if tag := strings.ToLower(c.Query("tag")); tag != "" {
		args = append(args, tag)
		q += ` AND $4 = ANY(s.tags)`
	}
	rows, err := s.db.Query(c, q+` ORDER BY s.key`, args...)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	secrets := []secretRow{}
	folders := map[string]bool{}
	for rows.Next() {
		r, err := s.scanSecret(rows)
		if err != nil {
			continue
		}
		if r.Path == path {
			secrets = append(secrets, *r)
			continue
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(r.Path, strings.TrimRight(path, "/")), "/")
		folders[strings.SplitN(rest, "/", 2)[0]] = true
	}
	fl := make([]string, 0, len(folders))
	for f := range folders {
		fl = append(fl, f)
	}
	sort.Strings(fl)
	c.JSON(200, gin.H{"secrets": secrets, "folders": fl, "path": path})
}

func (s *Server) searchSecrets(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(200, gin.H{"results": []secretRow{}})
		return
	}
	readable := s.readableEnvs(c, p)
	ids := []uuid.UUID{}
	for slug, id := range readable {
		if want := c.Query("env"); want == "" || want == slug {
			ids = append(ids, id)
		}
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	rows, err := s.db.Query(c, `SELECT `+secretCols+`, e.slug FROM secrets s JOIN environments e ON e.id=s.env_id
		WHERE s.project_id=$1 AND s.env_id = ANY($2) AND (s.key ILIKE $3 OR s.comment ILIKE $3 OR s.path ILIKE $3 OR EXISTS (SELECT 1 FROM unnest(s.tags) t WHERE t ILIKE $3))
		ORDER BY s.key, e.position LIMIT 100`, p.ID, ids, like)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []secretRow{}
	for rows.Next() {
		var env string
		r, err := s.scanSecret(rows, &env)
		if err != nil {
			continue
		}
		r.Env = env
		out = append(out, *r)
	}
	c.JSON(200, gin.H{"results": out})
}

func (s *Server) loadSecret(c *gin.Context, p *projectCtx) (*secretRow, []byte, bool) {
	sid, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		fail(c, 404, "not found")
		return nil, nil, false
	}
	var ct []byte
	var env string
	r, err := s.scanSecret(s.db.QueryRow(c, `SELECT `+secretCols+`, s.encrypted_value, e.slug FROM secrets s JOIN environments e ON e.id=s.env_id
		WHERE s.id=$1 AND s.project_id=$2`, sid, p.ID), &ct, &env)
	if err != nil {
		fail(c, 404, "not found")
		return nil, nil, false
	}
	r.Env = env
	return r, ct, true
}

func (s *Server) userAllow(c *gin.Context, p *projectCtx) func(env, path string) bool {
	cache := map[string]bool{}
	return func(env, _ string) bool {
		if v, ok := cache[env]; ok {
			return v
		}
		v := envRank[s.envRole(c, p, me(c).UserID, env)] >= envRank["viewer"]
		cache[env] = v
		return v
	}
}

func (s *Server) getSecret(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	r, ct, ok := s.loadSecret(c, p)
	if !ok || !s.needEnv(c, p, r.Env, "viewer") {
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	v, err := s.decrypt(dek, p.ID, ct)
	if err != nil {
		internalErr(c, err)
		return
	}
	r.Value = &v
	if strings.Contains(v, "${") {
		res := s.newResolver(c, p.ID, dek, s.userAllow(c, p)).expand(v, r.Env, r.Path, map[string]bool{r.Env + "|" + r.Path + "|" + r.Key: true}, 0)
		if res != v {
			r.Resolved = &res
		}
	}
	s.record(c, p.OrgID, &p.ID, "secret.read", join(r.Path, r.Key), map[string]any{"env": r.Env})
	c.JSON(200, r)
}

type putBody struct {
	Env         string     `json:"env"`
	Path        string     `json:"path"`
	Key         string     `json:"key"`
	Value       string     `json:"value"`
	Comment     string     `json:"comment"`
	Tags        *[]string  `json:"tags"`
	ExpiresAt   *time.Time `json:"expires_at"`
	ClearExpiry bool       `json:"clear_expiry"`
}

func (s *Server) putSecret(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	var in putBody
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "invalid request")
		return
	}
	path, pok := normPath(in.Path)
	if !pok || !keyRe.MatchString(in.Key) {
		fail(c, 400, "invalid key or path (keys: letters, digits, _ . -)")
		return
	}
	if len(in.Value) > maxValueLen {
		fail(c, 400, "value too large")
		return
	}
	if in.ExpiresAt != nil && in.ExpiresAt.Before(time.Now()) {
		fail(c, 400, "expiry must be in the future")
		return
	}
	if in.Tags != nil {
		t, ok := normTags(*in.Tags)
		if !ok {
			fail(c, 400, "tags: up to 10, lowercase letters, digits, - and _")
			return
		}
		in.Tags = &t
	}
	if !s.needEnv(c, p, in.Env, "developer") {
		return
	}
	eid, err := s.envID(c, p.ID, in.Env)
	if err != nil {
		fail(c, 404, "environment not found")
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	r, created, err := s.writeSecret(c, writeInput{ProjectID: p.ID, EnvID: eid, DEK: dek, Path: path, Key: in.Key, Value: in.Value,
		Comment: in.Comment, Actor: actorLabel(c), Tags: in.Tags, Expires: in.ExpiresAt, ClearExpiry: in.ClearExpiry})
	if err != nil {
		internalErr(c, err)
		return
	}
	act := "secret.update"
	if created {
		act = "secret.create"
	}
	s.record(c, p.OrgID, &p.ID, act, join(path, in.Key), map[string]any{"env": in.Env, "version": r.Version})
	r.Env = in.Env
	c.JSON(200, r)
}

func (s *Server) deleteSecret(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	r, _, ok := s.loadSecret(c, p)
	if !ok || !s.needEnv(c, p, r.Env, "developer") {
		return
	}
	s.db.Exec(c, `DELETE FROM secrets WHERE id=$1`, r.ID)
	s.record(c, p.OrgID, &p.ID, "secret.delete", join(r.Path, r.Key), map[string]any{"env": r.Env})
	c.Status(204)
}

func (s *Server) secretVersions(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	r, _, ok := s.loadSecret(c, p)
	if !ok || !s.needEnv(c, p, r.Env, "viewer") {
		return
	}
	rows, err := s.db.Query(c, `SELECT version,action,actor,created_at FROM secret_versions WHERE secret_id=$1 ORDER BY version DESC`, r.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var v int
		var action, actor string
		var at time.Time
		if rows.Scan(&v, &action, &actor, &at) == nil {
			out = append(out, gin.H{"version": v, "action": action, "actor": s.openLabel(actor), "at": at})
		}
	}
	c.JSON(200, out)
}

func (s *Server) rollbackSecret(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	r, _, ok := s.loadSecret(c, p)
	if !ok || !s.needEnv(c, p, r.Env, "developer") {
		return
	}
	var in struct{ Version int }
	if c.ShouldBindJSON(&in) != nil || in.Version < 1 {
		fail(c, 400, "version is required")
		return
	}
	var ct []byte
	if s.db.QueryRow(c, `SELECT encrypted_value FROM secret_versions WHERE secret_id=$1 AND version=$2`, r.ID, in.Version).Scan(&ct) != nil {
		fail(c, 404, "version not found")
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	val, err := s.decrypt(dek, p.ID, ct)
	if err != nil {
		internalErr(c, err)
		return
	}
	eid, _ := s.envID(c, p.ID, r.Env)
	nr, _, err := s.writeSecret(c, writeInput{ProjectID: p.ID, EnvID: eid, DEK: dek, Path: r.Path, Key: r.Key, Value: val, Comment: r.Comment, Actor: actorLabel(c), Action: "rollback"})
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "secret.rollback", join(r.Path, r.Key), map[string]any{"env": r.Env, "to_version": in.Version, "version": nr.Version})
	c.JSON(200, nr)
}

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func randomString(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		x, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[x.Int64()]
	}
	return string(b), nil
}

func (s *Server) rotateSecret(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	r, _, ok := s.loadSecret(c, p)
	if !ok || !s.needEnv(c, p, r.Env, "developer") {
		return
	}
	var in struct{ Length int }
	_ = c.ShouldBindJSON(&in)
	if in.Length < 16 || in.Length > 256 {
		in.Length = 32
	}
	val, err := randomString(in.Length)
	dek, err2 := s.dek(c, p.ID)
	if err != nil || err2 != nil {
		internalErr(c, errors.Join(err, err2))
		return
	}
	eid, _ := s.envID(c, p.ID, r.Env)
	nr, _, err := s.writeSecret(c, writeInput{ProjectID: p.ID, EnvID: eid, DEK: dek, Path: r.Path, Key: r.Key, Value: val, Comment: r.Comment, Actor: actorLabel(c), Action: "rotate"})
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "secret.rotate", join(r.Path, r.Key), map[string]any{"env": r.Env, "version": nr.Version})
	nr.Value = &val
	c.JSON(200, nr)
}

func (s *Server) importSecrets(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	var in struct{ Env, Path, Content string }
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "invalid request")
		return
	}
	path, pok := normPath(in.Path)
	eid, err := s.envID(c, p.ID, in.Env)
	if !pok || err != nil {
		fail(c, 400, "invalid environment or path")
		return
	}
	if !s.needEnv(c, p, in.Env, "developer") {
		return
	}
	pairs := parseDotenv(in.Content)
	if len(pairs) == 0 {
		fail(c, 400, "no KEY=VALUE pairs found")
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	created, updated, skipped := 0, 0, 0
	for _, kv := range pairs {
		if !keyRe.MatchString(kv[0]) || len(kv[1]) > maxValueLen {
			skipped++
			continue
		}
		_, isNew, err := s.writeSecret(c, writeInput{ProjectID: p.ID, EnvID: eid, DEK: dek, Path: path, Key: kv[0], Value: kv[1], Actor: actorLabel(c)})
		if err != nil {
			internalErr(c, err)
			return
		}
		if isNew {
			created++
		} else {
			updated++
		}
	}
	s.record(c, p.OrgID, &p.ID, "secret.import", path, map[string]any{"env": in.Env, "created": created, "updated": updated})
	c.JSON(200, gin.H{"created": created, "updated": updated, "skipped": skipped})
}

func (s *Server) exportSecrets(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	slug := c.Query("env")
	path, pok := normPath(c.Query("path"))
	eid, err := s.envID(c, p.ID, slug)
	if !pok || err != nil {
		fail(c, 400, "invalid environment or path")
		return
	}
	if !s.needEnv(c, p, slug, "viewer") {
		return
	}
	pairs, err := s.collect(c, p.ID, eid, slug, path, fetchOpts{Recursive: c.Query("recursive") == "true", Resolve: c.Query("resolve") != "false", Allow: s.userAllow(c, p)})
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "secret.export", path, map[string]any{"env": slug, "count": len(pairs)})
	respondSecrets(c, pairs, c.DefaultQuery("format", "json"))
}

type fetchOpts struct {
	Recursive bool
	Resolve   bool
	Allow     func(env, path string) bool
}

// collect decrypts the live (non-expired) secrets at path, and below if
// recursive. Deeper paths win on key collisions.
func (s *Server) collect(ctx context.Context, projectID, envID uuid.UUID, envSlug, path string, o fetchOpts) ([][2]string, error) {
	live := ` AND (expires_at IS NULL OR expires_at > now())`
	q := `SELECT path, key, encrypted_value FROM secrets WHERE env_id=$1 AND path=$2` + live + ` ORDER BY key`
	args := []any{envID, path}
	if o.Recursive {
		q = `SELECT path, key, encrypted_value FROM secrets WHERE env_id=$1 AND (path=$2 OR path LIKE $3)` + live + ` ORDER BY length(path), key`
		args = append(args, strings.TrimRight(path, "/")+"/%")
	}
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dek, err := s.dek(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var res *resolver
	if o.Resolve {
		res = s.newResolver(ctx, projectID, dek, o.Allow)
	}
	idx := map[string]int{}
	var out [][2]string
	for rows.Next() {
		var sp, k string
		var ct []byte
		if err := rows.Scan(&sp, &k, &ct); err != nil {
			return nil, err
		}
		v, err := s.decrypt(dek, projectID, ct)
		if err != nil {
			return nil, err
		}
		if res != nil {
			v = res.expand(v, envSlug, sp, map[string]bool{envSlug + "|" + sp + "|" + k: true}, 0)
		}
		if i, seen := idx[k]; seen {
			out[i][1] = v
		} else {
			idx[k] = len(out)
			out = append(out, [2]string{k, v})
		}
	}
	return out, rows.Err()
}

func respondSecrets(c *gin.Context, pairs [][2]string, format string) {
	switch format {
	case "dotenv":
		var b strings.Builder
		for _, kv := range pairs {
			fmt.Fprintf(&b, "%s=%s\n", kv[0], dotenvQuote(kv[1]))
		}
		c.Data(200, "text/plain; charset=utf-8", []byte(b.String()))
	case "shell":
		var b strings.Builder
		for _, kv := range pairs {
			fmt.Fprintf(&b, "export %s='%s'\n", kv[0], strings.ReplaceAll(kv[1], "'", `'\''`))
		}
		c.Data(200, "text/plain; charset=utf-8", []byte(b.String()))
	default:
		m := make(map[string]string, len(pairs))
		for _, kv := range pairs {
			m[kv[0]] = kv[1]
		}
		c.JSON(200, gin.H{"secrets": m})
	}
}

func dotenvQuote(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t\n\r#\"'\\$`=") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

func parseDotenv(content string) [][2]string {
	var out [][2]string
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch {
		case len(v) >= 2 && v[0] == '"' && strings.HasSuffix(v, `"`):
			v = strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\"`, `"`, `\\`, `\`).Replace(v[1 : len(v)-1])
		case len(v) >= 2 && v[0] == '\'' && strings.HasSuffix(v, "'"):
			v = v[1 : len(v)-1]
		default:
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
		}
		out = append(out, [2]string{k, v})
	}
	return out
}
