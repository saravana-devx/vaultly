package routes

import (
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/middleware"
)

func registerAuth(api *gin.RouterGroup, h Handlers, m *middleware.Set) {
	api.POST("/auth/register", m.Limit("register", 10, time.Hour, middleware.ByIP), h.User.Register)
	api.POST("/auth/login", m.Limit("login", 10, time.Minute, middleware.ByIP), h.User.Login)
	api.POST("/auth/login/mfa", m.Limit("login-mfa", 10, time.Minute, middleware.ByIP), h.User.LoginMFA)
	api.POST("/auth/refresh", m.Limit("refresh", 60, time.Minute, middleware.ByIP), h.User.Refresh)
	api.POST("/auth/logout", h.User.Logout)
	api.GET("/invites/:token", m.Limit("invite", 30, time.Minute, middleware.ByIP), h.Invite.Preview)
}

func registerAccount(u *gin.RouterGroup, h Handlers) {
	u.GET("/me", h.User.Me)
	u.GET("/me/mfa", h.User.MFAStatus)
	u.POST("/me/mfa/setup", h.User.MFASetup)
	u.POST("/me/mfa/enable", h.User.MFAEnable)
	u.POST("/me/mfa/disable", h.User.MFADisable)
	u.POST("/me/mfa/recovery-codes", h.User.MFARegenerate)
	u.GET("/me/tokens", h.Token.ListPersonal)
	u.POST("/me/tokens", h.Token.CreatePersonal)
	u.DELETE("/me/tokens/:tid", h.Token.RevokePersonal)
	u.POST("/invites/:token/accept", h.Invite.Accept)
}
