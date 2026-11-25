package routes

import (
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/middleware"
)

func registerProject(u *gin.RouterGroup, h Handlers, m *middleware.Set) {
	u.GET("/projects/:pid", h.Project.Get)
	u.DELETE("/projects/:pid", h.Project.Delete)
	u.POST("/projects/:pid/environments", h.Project.CreateEnv)
	u.DELETE("/projects/:pid/environments/:eid", h.Project.DeleteEnv)

	u.GET("/projects/:pid/secrets", h.Secret.List)
	u.GET("/projects/:pid/secrets/search", h.Secret.Search)
	u.PUT("/projects/:pid/secrets", h.Secret.Put)
	u.POST("/projects/:pid/secrets/import", h.Secret.Import)
	u.GET("/projects/:pid/secrets/export", m.Limit("export", 10, time.Minute, middleware.ByUser), h.Secret.Export)
	u.GET("/projects/:pid/secrets/:sid", m.Limit("reveal", 60, time.Minute, middleware.ByUser), h.Secret.Get)
	u.DELETE("/projects/:pid/secrets/:sid", h.Secret.Delete)
	u.GET("/projects/:pid/secrets/:sid/versions", h.Secret.Versions)
	u.POST("/projects/:pid/secrets/:sid/rollback", h.Secret.Rollback)
	u.POST("/projects/:pid/secrets/:sid/rotate", h.Secret.Rotate)

	u.GET("/projects/:pid/tokens", h.Token.ListService)
	u.POST("/projects/:pid/tokens", h.Token.CreateService)
	u.DELETE("/projects/:pid/tokens/:tid", h.Token.RevokeService)

	u.GET("/projects/:pid/webhooks", h.Webhook.List)
	u.POST("/projects/:pid/webhooks", h.Webhook.Create)
	u.PATCH("/projects/:pid/webhooks/:wid", h.Webhook.Update)
	u.DELETE("/projects/:pid/webhooks/:wid", h.Webhook.Delete)
	u.POST("/projects/:pid/webhooks/:wid/test", m.Limit("webhook-test", 20, time.Minute, middleware.ByUser), h.Webhook.Test)
	u.GET("/projects/:pid/webhooks/:wid/deliveries", h.Webhook.Deliveries)

	u.GET("/projects/:pid/integrations", h.Integration.List)
	u.POST("/projects/:pid/integrations", h.Integration.Create)
	u.PATCH("/projects/:pid/integrations/:iid", h.Integration.Update)
	u.DELETE("/projects/:pid/integrations/:iid", h.Integration.Delete)
	u.POST("/projects/:pid/integrations/:iid/sync", m.Limit("sync", 20, time.Minute, middleware.ByUser), h.Integration.Sync)

	u.GET("/projects/:pid/dynamic", h.Dynamic.List)
	u.POST("/projects/:pid/dynamic", h.Dynamic.Create)
	u.DELETE("/projects/:pid/dynamic/:did", h.Dynamic.Delete)
	u.POST("/projects/:pid/dynamic/:did/lease", m.Limit("lease", 30, time.Minute, middleware.ByUser), h.Dynamic.Lease)
	u.GET("/projects/:pid/dynamic/:did/leases", h.Dynamic.Leases)
	u.DELETE("/projects/:pid/dynamic/:did/leases/:lid", h.Dynamic.Revoke)
}
