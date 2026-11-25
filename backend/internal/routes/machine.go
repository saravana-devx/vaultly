package routes

import (
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/middleware"
)

// registerMachine serves the CLI and CI: service tokens scoped to one environment and path.
func registerMachine(api *gin.RouterGroup, h Handlers, m *middleware.Set) {
	v1 := api.Group("/v1", m.ServiceAuth(), m.Limit("token", 600, time.Minute, middleware.ByToken))
	v1.GET("/secrets", h.Secret.Fetch)
	v1.PUT("/secrets", h.Secret.MachinePut)
	v1.POST("/dynamic/:name/lease", m.Limit("token-lease", 60, time.Minute, middleware.ByToken), h.Dynamic.TokenLease)
}
