// Package routes maps URLs to handlers. Each file registers one area of the API.
package routes

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/audit"
	"vaultly/internal/backup"
	"vaultly/internal/dynamic"
	"vaultly/internal/integration"
	"vaultly/internal/invite"
	"vaultly/internal/member"
	"vaultly/internal/middleware"
	"vaultly/internal/org"
	"vaultly/internal/project"
	"vaultly/internal/secret"
	"vaultly/internal/token"
	"vaultly/internal/user"
	"vaultly/internal/webhook"
)

type Handlers struct {
	User        *user.Handler
	Org         *org.Handler
	Member      *member.Handler
	Invite      *invite.Handler
	Project     *project.Handler
	Secret      *secret.Handler
	Token       *token.Handler
	Webhook     *webhook.Handler
	Integration *integration.Handler
	Dynamic     *dynamic.Handler
	Backup      *backup.Handler
	Audit       *audit.Handler
}

func Register(r *gin.Engine, h Handlers, m *middleware.Set) {
	r.Use(gin.Recovery(), gin.Logger(), m.CORS(), m.SecurityHeaders())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	api := r.Group("/api", m.Limit("ip", 600, time.Minute, middleware.ByIP))
	registerAuth(api, h, m)

	users := api.Group("", m.UserAuth(), m.Limit("user", 600, time.Minute, middleware.ByUser))
	registerAccount(users, h)
	registerWorkspace(users, h, m)
	registerProject(users, h, m)

	registerMachine(api, h, m)
}
