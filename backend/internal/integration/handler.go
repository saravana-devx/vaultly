package integration

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.List(ctx, principal.From(c), pid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgListed, out)
}

func (h *Handler) Create(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	var body CreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Create(ctx, principal.From(c), pid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgCreated, out)
}

func (h *Handler) Update(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	iid, ok2 := httpx.UUIDParam(c, "iid")
	if !ok || !ok2 {
		return
	}
	var body UpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.SetAutoSync(ctx, principal.From(c), pid, iid, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgUpdated, nil)
}

func (h *Handler) Delete(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	iid, ok2 := httpx.UUIDParam(c, "iid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.Delete(ctx, principal.From(c), pid, iid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgDeleted, nil)
}

func (h *Handler) Sync(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	iid, ok2 := httpx.UUIDParam(c, "iid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, 90*time.Second)
	defer cancel()
	out, err := h.svc.SyncNow(ctx, principal.From(c), pid, iid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSynced, out)
}
