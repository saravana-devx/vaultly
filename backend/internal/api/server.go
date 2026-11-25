package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/saravana/vaultly/internal/audit"
	"github.com/saravana/vaultly/internal/auth"
	"github.com/saravana/vaultly/internal/config"
	"github.com/saravana/vaultly/internal/crypto"
	"github.com/saravana/vaultly/internal/dynamic"
	"github.com/saravana/vaultly/internal/integrations"
	"github.com/saravana/vaultly/internal/mail"
	"github.com/saravana/vaultly/internal/pii"
	"github.com/saravana/vaultly/internal/ratelimit"
	"github.com/saravana/vaultly/internal/webhook"
)

type Server struct {
	cfg       *config.Config
	db        *pgxpool.Pool
	rdb       *redis.Client
	keys      crypto.KeyProvider
	pii       *pii.Box
	deks      *dekCache
	auth      *auth.Manager
	limiter   ratelimit.Limiter
	mail      mail.Sender
	hooks     *webhook.Dispatcher
	providers map[string]integrations.Provider
	dyn       dynamic.Provider
	sync      *syncState
	audit     *audit.Service
	dummyHash string
}

func NewServer(cfg *config.Config, db *pgxpool.Pool, rdb *redis.Client, keys crypto.KeyProvider, box *pii.Box, a *audit.Service) *Server {
	dh, _ := auth.HashPassword("dummy-password-for-timing")
	s := &Server{cfg: cfg, db: db, rdb: rdb, keys: keys, pii: box, deks: newDEKCache(cfg.KeyCacheTTL), auth: auth.NewManager(cfg.JWTSecret, rdb), limiter: ratelimit.NewSlidingWindow(rdb), audit: a, dummyHash: dh,
		mail:      mail.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom),
		hooks:     webhook.New(db, keys, cfg.WebhookAllowPrivate),
		providers: integrations.Registry(nil),
		dyn:       dynamic.Postgres{},
		sync:      &syncState{timers: map[uuid.UUID]*time.Timer{}},
	}
	a.Subscribe(s.hooks.Handle)
	a.Subscribe(s.autoSync)
	return s
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.Use(gin.Recovery(), gin.Logger(), s.cors(), securityHeaders())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api", s.rateLimit("ip", 600, time.Minute, byIP))
	api.POST("/auth/register", s.rateLimit("register", 10, time.Hour, byIP), s.register)
	api.POST("/auth/login", s.rateLimit("login", 10, time.Minute, byIP), s.login)
	api.POST("/auth/refresh", s.rateLimit("refresh", 60, time.Minute, byIP), s.refresh)
	api.POST("/auth/logout", s.logout)
	api.GET("/invites/:token", s.rateLimit("invite", 30, time.Minute, byIP), s.previewInvite)

	u := api.Group("", s.userAuth(), s.rateLimit("user", 600, time.Minute, byUser))
	u.GET("/me", s.me)
	u.GET("/me/tokens", s.listPersonalTokens)
	u.POST("/me/tokens", s.createPersonalToken)
	u.DELETE("/me/tokens/:tid", s.deletePersonalToken)
	u.GET("/orgs", s.listOrgs)
	u.POST("/orgs", s.createOrg)

	u.GET("/orgs/:org/stats", s.orgStats)
	u.GET("/orgs/:org/members", s.listMembers)
	u.POST("/orgs/:org/members", s.addMember)
	u.PATCH("/orgs/:org/members/:uid", s.updateMember)
	u.DELETE("/orgs/:org/members/:uid", s.removeMember)
	u.GET("/orgs/:org/env-slugs", s.envSlugs)
	u.GET("/orgs/:org/members/:uid/env-roles", s.getMemberEnvRoles)
	u.PUT("/orgs/:org/members/:uid/env-roles", s.putMemberEnvRoles)
	u.GET("/orgs/:org/invites", s.listInvites)
	u.POST("/orgs/:org/invites", s.createInvite)
	u.DELETE("/orgs/:org/invites/:iid", s.revokeInvite)
	u.POST("/invites/:token/accept", s.acceptInvite)
	u.GET("/orgs/:org/projects", s.listProjects)
	u.POST("/orgs/:org/projects", s.createProject)
	u.GET("/orgs/:org/audit", s.listAudit)

	u.GET("/projects/:pid", s.getProject)
	u.DELETE("/projects/:pid", s.deleteProject)
	u.POST("/projects/:pid/environments", s.createEnv)
	u.DELETE("/projects/:pid/environments/:eid", s.deleteEnv)

	u.GET("/projects/:pid/secrets", s.listSecrets)
	u.GET("/projects/:pid/secrets/search", s.searchSecrets)
	u.PUT("/projects/:pid/secrets", s.putSecret)
	u.POST("/projects/:pid/secrets/import", s.importSecrets)
	u.GET("/projects/:pid/secrets/export", s.rateLimit("export", 10, time.Minute, byUser), s.exportSecrets)
	u.GET("/projects/:pid/secrets/:sid", s.rateLimit("reveal", 60, time.Minute, byUser), s.getSecret)
	u.DELETE("/projects/:pid/secrets/:sid", s.deleteSecret)
	u.GET("/projects/:pid/secrets/:sid/versions", s.secretVersions)
	u.POST("/projects/:pid/secrets/:sid/rollback", s.rollbackSecret)
	u.POST("/projects/:pid/secrets/:sid/rotate", s.rotateSecret)

	u.GET("/projects/:pid/webhooks", s.listWebhooks)
	u.POST("/projects/:pid/webhooks", s.createWebhook)
	u.PATCH("/projects/:pid/webhooks/:wid", s.updateWebhook)
	u.DELETE("/projects/:pid/webhooks/:wid", s.deleteWebhook)
	u.POST("/projects/:pid/webhooks/:wid/test", s.rateLimit("webhook-test", 20, time.Minute, byUser), s.testWebhook)
	u.GET("/projects/:pid/webhooks/:wid/deliveries", s.webhookDeliveries)

	u.GET("/projects/:pid/integrations", s.listIntegrations)
	u.POST("/projects/:pid/integrations", s.createIntegration)
	u.PATCH("/projects/:pid/integrations/:iid", s.updateIntegration)
	u.DELETE("/projects/:pid/integrations/:iid", s.deleteIntegration)
	u.POST("/projects/:pid/integrations/:iid/sync", s.rateLimit("sync", 20, time.Minute, byUser), s.syncNow)

	u.GET("/projects/:pid/dynamic", s.listDynamic)
	u.POST("/projects/:pid/dynamic", s.createDynamic)
	u.DELETE("/projects/:pid/dynamic/:did", s.deleteDynamic)
	u.POST("/projects/:pid/dynamic/:did/lease", s.rateLimit("lease", 30, time.Minute, byUser), s.createLease)
	u.GET("/projects/:pid/dynamic/:did/leases", s.listLeases)
	u.DELETE("/projects/:pid/dynamic/:did/leases/:lid", s.revokeLease)

	u.POST("/orgs/:org/backup", s.rateLimit("backup", 5, time.Hour, byUser), s.createBackup)
	u.POST("/orgs/:org/restore", s.rateLimit("restore", 5, time.Hour, byUser), s.restoreBackup)

	u.GET("/projects/:pid/tokens", s.listTokens)
	u.POST("/projects/:pid/tokens", s.createToken)
	u.DELETE("/projects/:pid/tokens/:tid", s.deleteToken)

	v1 := api.Group("/v1", s.tokenAuth(), s.rateLimit("token", 600, time.Minute, byToken))
	v1.GET("/secrets", s.tokenGetSecrets)
	v1.PUT("/secrets", s.tokenPutSecret)
	v1.POST("/dynamic/:name/lease", s.rateLimit("token-lease", 60, time.Minute, byToken), s.tokenLease)
	return r
}

// ---- middleware ----

func (s *Server) cors() gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, o := range s.cfg.CORSOrigins {
		allowed[strings.TrimSpace(o)] = true
	}
	return func(c *gin.Context) {
		if o := c.GetHeader("Origin"); o != "" && allowed[o] {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

type principal struct {
	UserID uuid.UUID
	Email  string
	ViaPAT bool
	Token  *tokenInfo
}

type tokenInfo struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	OrgID      uuid.UUID
	EnvID      uuid.UUID
	EnvSlug    string
	Name       string
	PathPrefix string
	CanWrite   bool
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (s *Server) userAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c)
		if strings.HasPrefix(raw, personalPrefix) {
			hash := sha256.Sum256([]byte(raw))
			var id uuid.UUID
			var email string
			var exp *time.Time
			var tid uuid.UUID
			var emailEnc string
			err := s.db.QueryRow(c, `SELECT u.id, u.email_enc, t.expires_at, t.id FROM personal_tokens t JOIN users u ON u.id=t.user_id WHERE t.token_hash=$1`, hash[:]).
				Scan(&id, &emailEnc, &exp, &tid)
			if err == nil {
				email, err = s.pii.Open("users.email", emailEnc)
			}
			if err != nil || (exp != nil && exp.Before(time.Now())) {
				fail(c, 401, "invalid or expired token")
				return
			}
			go s.db.Exec(context.Background(), `UPDATE personal_tokens SET last_used_at = now() WHERE id = $1`, tid)
			c.Set("principal", &principal{UserID: id, Email: email, ViaPAT: true})
			c.Next()
			return
		}
		claims, err := s.auth.ParseAccessToken(raw)
		if err != nil {
			fail(c, 401, "unauthorized")
			return
		}
		id, err := uuid.Parse(claims.Subject)
		if err != nil {
			fail(c, 401, "unauthorized")
			return
		}
		c.Set("principal", &principal{UserID: id, Email: claims.Email})
		c.Next()
	}
}

func (s *Server) tokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c)
		if !strings.HasPrefix(raw, tokenPrefix) {
			fail(c, 401, "a service token is required")
			return
		}
		hash := sha256.Sum256([]byte(raw))
		t := &tokenInfo{}
		var exp *time.Time
		var cidrs []string
		err := s.db.QueryRow(c, `
			SELECT t.id, t.project_id, p.org_id, t.env_id, e.slug, t.name, t.path_prefix, t.can_write, t.allowed_cidrs, t.expires_at
			FROM service_tokens t JOIN projects p ON p.id = t.project_id JOIN environments e ON e.id = t.env_id
			WHERE t.token_hash = $1`, hash[:]).
			Scan(&t.ID, &t.ProjectID, &t.OrgID, &t.EnvID, &t.EnvSlug, &t.Name, &t.PathPrefix, &t.CanWrite, &cidrs, &exp)
		if err != nil || (exp != nil && exp.Before(time.Now())) {
			fail(c, 401, "invalid or expired token")
			return
		}
		if !ipAllowed(cidrs, c.ClientIP()) {
			s.audit.Publish(c, audit.Event{OrgID: t.OrgID, ProjectID: &t.ProjectID, ActorType: "token", ActorID: t.ID.String(), ActorLabel: "token:" + t.Name,
				Action: "token.denied", Resource: t.Name, Metadata: map[string]any{"reason": "ip_not_allowed"}, IP: c.ClientIP()})
			fail(c, 403, "this token cannot be used from your IP address")
			return
		}
		go s.db.Exec(context.Background(), `UPDATE service_tokens SET last_used_at = now() WHERE id = $1`, t.ID)
		c.Set("principal", &principal{Token: t})
		c.Next()
	}
}

func me(c *gin.Context) *principal { return c.MustGet("principal").(*principal) }

// ---- helpers ----

func fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

func internalErr(c *gin.Context, err error) {
	_ = c.Error(err)
	fail(c, 500, "internal error")
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

var roleRank = map[string]int{"viewer": 1, "developer": 2, "admin": 3}

func validRole(r string) bool { _, ok := roleRank[r]; return ok }

// orgRole returns the caller's role in the org from the :org param, or writes an error.
func (s *Server) orgRole(c *gin.Context, need string) (uuid.UUID, string, bool) {
	orgID, err := uuid.Parse(c.Param("org"))
	if err != nil {
		fail(c, 404, "not found")
		return uuid.Nil, "", false
	}
	var role string
	err = s.db.QueryRow(c, `SELECT role FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, me(c).UserID).Scan(&role)
	if err != nil {
		fail(c, 404, "not found") // don't leak existence
		return uuid.Nil, "", false
	}
	if roleRank[role] < roleRank[need] {
		fail(c, 403, "requires "+need+" role")
		return uuid.Nil, "", false
	}
	return orgID, role, true
}

type projectCtx struct {
	ID    uuid.UUID
	OrgID uuid.UUID
	Name  string
	Role  string
}

func (s *Server) project(c *gin.Context, need string) (*projectCtx, bool) {
	pid, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		fail(c, 404, "not found")
		return nil, false
	}
	p := &projectCtx{}
	err = s.db.QueryRow(c, `
		SELECT p.id, p.org_id, p.name, m.role FROM projects p
		JOIN memberships m ON m.org_id = p.org_id AND m.user_id = $2
		WHERE p.id = $1`, pid, me(c).UserID).Scan(&p.ID, &p.OrgID, &p.Name, &p.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, 404, "not found")
		return nil, false
	} else if err != nil {
		internalErr(c, err)
		return nil, false
	}
	if roleRank[p.Role] < roleRank[need] {
		fail(c, 403, "requires "+need+" role")
		return nil, false
	}
	return p, true
}

func (s *Server) envID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT id FROM environments WHERE project_id=$1 AND slug=$2`, projectID, slug).Scan(&id)
	return id, err
}

func (s *Server) dek(ctx context.Context, projectID uuid.UUID) ([]byte, error) {
	if d, ok := s.deks.get(projectID); ok {
		return d, nil
	}
	var wrapped []byte
	if err := s.db.QueryRow(ctx, `SELECT encrypted_dek FROM projects WHERE id=$1`, projectID).Scan(&wrapped); err != nil {
		return nil, err
	}
	d, err := s.keys.UnwrapDEK(ctx, wrapped)
	if err == nil {
		s.deks.put(projectID, d)
	}
	return d, err
}

func (s *Server) record(c *gin.Context, orgID uuid.UUID, projectID *uuid.UUID, action, resource string, md map[string]any) {
	p := me(c)
	e := audit.Event{OrgID: orgID, ProjectID: projectID, Action: action, Resource: resource, Metadata: md, IP: c.ClientIP()}
	if p.Token != nil {
		e.ActorType, e.ActorID, e.ActorLabel = "token", p.Token.ID.String(), "token:"+p.Token.Name
	} else {
		e.ActorType, e.ActorID, e.ActorLabel = "user", p.UserID.String(), p.Email
		if p.ViaPAT {
			if e.Metadata == nil {
				e.Metadata = map[string]any{}
			}
			e.Metadata["via"] = "personal_token"
		}
	}
	s.audit.Publish(c, e)
}

func actorLabel(c *gin.Context) string {
	p := me(c)
	if p.Token != nil {
		return "token:" + p.Token.Name
	}
	return p.Email
}
