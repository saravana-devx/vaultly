package routes

import (
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/middleware"
)

func registerWorkspace(u *gin.RouterGroup, h Handlers, m *middleware.Set) {
	u.GET("/orgs", h.Org.List)
	u.POST("/orgs", h.Org.Create)
	u.GET("/orgs/:org/stats", h.Org.Stats)
	u.GET("/orgs/:org/env-slugs", h.Org.EnvSlugs)
	u.GET("/orgs/:org/audit", h.Audit.List)

	u.GET("/orgs/:org/members", h.Member.List)
	u.POST("/orgs/:org/members", h.Member.Add)
	u.PATCH("/orgs/:org/members/:uid", h.Member.UpdateRole)
	u.DELETE("/orgs/:org/members/:uid", h.Member.Remove)
	u.GET("/orgs/:org/members/:uid/env-roles", h.Member.GetEnvRoles)
	u.PUT("/orgs/:org/members/:uid/env-roles", h.Member.PutEnvRoles)

	u.GET("/orgs/:org/invites", h.Invite.List)
	u.POST("/orgs/:org/invites", h.Invite.Create)
	u.DELETE("/orgs/:org/invites/:iid", h.Invite.Revoke)

	u.GET("/orgs/:org/projects", h.Project.List)
	u.POST("/orgs/:org/projects", h.Project.Create)

	u.POST("/orgs/:org/backup", m.Limit("backup", 5, time.Hour, middleware.ByUser), h.Backup.Create)
	u.POST("/orgs/:org/restore", m.Limit("restore", 5, time.Hour, middleware.ByUser), h.Backup.Restore)
}
