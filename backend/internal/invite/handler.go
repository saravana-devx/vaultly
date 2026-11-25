package invite

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Create(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	var body CreateInviteRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	created, err := h.svc.Create(ctx, principal.From(c), org, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgInviteCreated, created)
}

func (h *Handler) List(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	invites, err := h.svc.List(ctx, principal.From(c), org)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgInvitesFetched, invites)
}

func (h *Handler) Revoke(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	id, ok2 := httpx.UUIDParam(c, "iid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.Revoke(ctx, principal.From(c), org, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgInviteRevoked, nil)
}

func (h *Handler) Preview(c *gin.Context) {
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	preview, err := h.svc.Preview(ctx, c.Param("token"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgInviteFetched, preview)
}

func (h *Handler) Accept(c *gin.Context) {
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	accepted, err := h.svc.Accept(ctx, principal.From(c), c.Param("token"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgInviteAccepted, accepted)
}
