package token

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) ListService(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.ListService(ctx, principal.From(c), pid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgServiceListed, out)
}

func (h *Handler) CreateService(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	var body CreateServiceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.CreateService(ctx, principal.From(c), pid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgServiceCreated, out)
}

func (h *Handler) RevokeService(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	tid, ok2 := httpx.UUIDParam(c, "tid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.RevokeService(ctx, principal.From(c), pid, tid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgServiceRevoked, nil)
}

func (h *Handler) ListPersonal(c *gin.Context) {
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.ListPersonal(ctx, principal.From(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgPersonalListed, out)
}

func (h *Handler) CreatePersonal(c *gin.Context) {
	var body CreatePersonalRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.CreatePersonal(ctx, principal.From(c), &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgPersonalCreated, out)
}

func (h *Handler) RevokePersonal(c *gin.Context) {
	tid, ok := httpx.UUIDParam(c, "tid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.RevokePersonal(ctx, principal.From(c), tid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgPersonalRevoked, nil)
}

// WhoAmI describes the calling service token, so the CLI can confirm a login and show its scope.
func (h *Handler) WhoAmI(c *gin.Context) {
	t := principal.From(c).Token
	httpx.Success(c, http.StatusOK, MsgWhoAmI, gin.H{
		"name": t.Name, "project_id": t.ProjectID, "env": t.EnvSlug, "path": t.PathPrefix, "can_write": t.CanWrite,
	})
}
