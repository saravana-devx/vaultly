package member

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	members, err := h.svc.List(ctx, principal.From(c), org)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgMembersFetched, members)
}

func (h *Handler) Add(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	var body AddMemberRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.Add(ctx, principal.From(c), org, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgMemberAdded, nil)
}

func (h *Handler) UpdateRole(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	target, ok2 := httpx.UUIDParam(c, "uid")
	if !ok || !ok2 {
		return
	}
	var body UpdateRoleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.UpdateRole(ctx, principal.From(c), org, target, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgRoleUpdated, nil)
}

func (h *Handler) Remove(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	target, ok2 := httpx.UUIDParam(c, "uid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.Remove(ctx, principal.From(c), org, target); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgMemberRemoved, nil)
}

func (h *Handler) GetEnvRoles(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	target, ok2 := httpx.UUIDParam(c, "uid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	roles, err := h.svc.EnvRoles(ctx, principal.From(c), org, target)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgEnvRolesFetched, EnvRolesResponse{Roles: roles})
}

func (h *Handler) PutEnvRoles(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	target, ok2 := httpx.UUIDParam(c, "uid")
	if !ok || !ok2 {
		return
	}
	var body EnvRolesRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.SetEnvRoles(ctx, principal.From(c), org, target, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgEnvRolesUpdated, nil)
}
