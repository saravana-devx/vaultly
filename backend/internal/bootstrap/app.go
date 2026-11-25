// Package bootstrap wires configuration, infrastructure and every domain into one App.
package bootstrap

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/auth"
	"vaultly/internal/backup"
	"vaultly/internal/config"
	"vaultly/internal/crypto"
	"vaultly/internal/dynamic"
	"vaultly/internal/integration"
	"vaultly/internal/invite"
	"vaultly/internal/janitor"
	"vaultly/internal/keystore"
	"vaultly/internal/mail"
	"vaultly/internal/member"
	"vaultly/internal/middleware"
	"vaultly/internal/org"
	"vaultly/internal/pii"
	"vaultly/internal/project"
	"vaultly/internal/ratelimit"
	"vaultly/internal/routes"
	"vaultly/internal/secret"
	"vaultly/internal/token"
	"vaultly/internal/user"
	"vaultly/internal/webhook"
)

type Deps struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Keys   crypto.KeyProvider
	PII    *pii.Box
	Audit  *audit.Service
}

type App struct {
	router  *gin.Engine
	janitor *janitor.Janitor
	port    string
}

func New(d Deps) *App {
	cfg, db := d.Config, d.DB

	accessSvc := access.NewService(access.NewRepository(db))
	keys := keystore.NewService(d.Keys, keystore.NewRepository(db), cfg.KeyCacheTTL)
	sessions := auth.NewManager(cfg.JWTSecret, d.Redis)

	users := user.NewService(user.NewRepository(db), d.PII, sessions, d.Audit)
	tokens := token.NewService(token.NewRepository(db), accessSvc, d.PII, d.Audit)
	secrets := secret.NewService(secret.NewRepository(db), accessSvc, keys, d.PII, d.Audit)

	dyn := dynamic.NewService(dynamic.NewRepository(db), accessSvc, keys, d.PII, dynamic.Postgres{}, d.Audit)

	hookRepo := webhook.NewRepository(db)
	dispatcher := webhook.NewDispatcher(hookRepo, keys, cfg.WebhookAllowPrivate)
	hooks := webhook.NewService(hookRepo, accessSvc, keys, dispatcher, d.Audit, cfg.WebhookAllowPrivate)

	integrations := integration.NewService(integration.NewRepository(db), accessSvc, keys, secrets, integration.Registry(nil), d.Audit)

	d.Audit.Subscribe(dispatcher.Handle)
	d.Audit.Subscribe(integrations.AutoSync)

	mailer := mail.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)

	h := routes.Handlers{
		User:        user.NewHandler(users),
		Org:         org.NewHandler(org.NewService(org.NewRepository(db), accessSvc, d.PII)),
		Member:      member.NewHandler(member.NewService(member.NewRepository(db), accessSvc, users, d.Audit)),
		Invite:      invite.NewHandler(invite.NewService(invite.NewRepository(db), accessSvc, users, d.PII, mailer, d.Audit, cfg.AppURL)),
		Project:     project.NewHandler(project.NewService(project.NewRepository(db), accessSvc, keys, dyn, d.Audit)),
		Secret:      secret.NewHandler(secrets),
		Token:       token.NewHandler(tokens),
		Webhook:     webhook.NewHandler(hooks),
		Integration: integration.NewHandler(integrations),
		Dynamic:     dynamic.NewHandler(dyn),
		Backup:      backup.NewHandler(backup.NewService(backup.NewRepository(db), accessSvc, keys, d.PII, d.Audit)),
		Audit:       audit.NewHandler(audit.NewReader(db, accessSvc, d.PII)),
	}

	mw := middleware.New(sessions, tokens, newLimiter(cfg, d.Redis), cfg.CORSOrigins)
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	routes.Register(r, h, mw)

	jan := janitor.New(20*time.Second,
		janitor.JobFunc(func(ctx context.Context) { dyn.RevokeExpired(ctx) }),
		janitor.JobFunc(secrets.AnnounceExpired),
	)
	return &App{router: r, janitor: jan, port: cfg.Port}
}

// newLimiter uses the standalone Distributed-rate-limiter service when configured and falls
// back to the in-process Redis sliding window.
func newLimiter(cfg *config.Config, rdb *redis.Client) ratelimit.Limiter {
	local := ratelimit.NewSlidingWindow(rdb)
	if cfg.RateLimiterURL == "" {
		return local
	}
	return ratelimit.NewRemote(cfg.RateLimiterURL, local)
}

// Run starts the housekeeping jobs and serves HTTP until ctx is cancelled.
func (a *App) Run(ctx context.Context, logf func(string, ...any)) error {
	go a.janitor.Run(ctx)
	srv := &http.Server{Addr: ":" + a.port, Handler: a.router, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		logf("vaultly api listening on :%s", a.port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errc <- err
		}
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(sctx)
}
