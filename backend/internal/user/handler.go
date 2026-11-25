package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"vaultly/internal/auth"
	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

const refreshCookie = "vaultly_rt"

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(c *gin.Context) {
	var body RegisterRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	session, err := h.svc.Register(ctx, &body, c.ClientIP())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respond(c, http.StatusCreated, MsgRegistered, session)
}

func (h *Handler) Login(c *gin.Context) {
	var body LoginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	session, err := h.svc.Login(ctx, &body, c.ClientIP())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, MsgLoggedIn, session)
}

func (h *Handler) Refresh(c *gin.Context) {
	token, _ := c.Cookie(refreshCookie)
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	session, err := h.svc.Refresh(ctx, token)
	if err != nil {
		if err == ErrSessionExpired {
			setRefreshCookie(c, "", -1)
		}
		httpx.Fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, MsgSessionRefreshed, session)
}

func (h *Handler) Logout(c *gin.Context) {
	token, _ := c.Cookie(refreshCookie)
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	h.svc.Logout(ctx, token)
	setRefreshCookie(c, "", -1)
	httpx.Success(c, http.StatusOK, MsgLoggedOut, nil)
}

func (h *Handler) Me(c *gin.Context) {
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	profile, err := h.svc.Profile(ctx, principal.From(c).UserID)
	if err != nil {
		httpx.Fail(c, ErrSessionExpired)
		return
	}
	httpx.Success(c, http.StatusOK, MsgProfileFetched, profile)
}

func (h *Handler) respond(c *gin.Context, status int, message string, s *Session) {
	setRefreshCookie(c, s.RefreshToken, int(auth.RefreshTTL.Seconds()))
	httpx.Success(c, status, message, AuthResponse{AccessToken: s.AccessToken, ExpiresIn: int(auth.AccessTTL.Seconds()), User: s.User})
}

func setRefreshCookie(c *gin.Context, token string, maxAge int) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	http.SetCookie(c.Writer, &http.Cookie{
		Name: refreshCookie, Value: token, Path: "/api/auth", MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}
