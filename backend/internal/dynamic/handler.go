package dynamic

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
	ctx, cancel := httpx.Timeout(c, 20*time.Second)
	defer cancel()
	out, err := h.svc.Create(ctx, principal.From(c), pid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgCreated, out)
}

func (h *Handler) Delete(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	did, ok2 := httpx.UUIDParam(c, "did")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, 60*time.Second)
	defer cancel()
	if err := h.svc.Delete(ctx, principal.From(c), pid, did); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgDeleted, nil)
}

func (h *Handler) Lease(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	did, ok2 := httpx.UUIDParam(c, "did")
	if !ok || !ok2 {
		return
	}
	var body LeaseRequest
	_ = c.ShouldBindJSON(&body)
	ctx, cancel := httpx.Timeout(c, 30*time.Second)
	defer cancel()
	out, err := h.svc.Lease(ctx, principal.From(c), pid, did, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgLeased, out)
}

func (h *Handler) Leases(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	did, ok2 := httpx.UUIDParam(c, "did")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Leases(ctx, principal.From(c), pid, did)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgLeasesListed, out)
}

func (h *Handler) Revoke(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	lid, ok2 := httpx.UUIDParam(c, "lid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, 30*time.Second)
	defer cancel()
	if err := h.svc.Revoke(ctx, principal.From(c), pid, lid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgRevoked, nil)
}

// TokenLease is the machine endpoint (service token).
func (h *Handler) TokenLease(c *gin.Context) {
	var body LeaseRequest
	_ = c.ShouldBindJSON(&body)
	ctx, cancel := httpx.Timeout(c, 30*time.Second)
	defer cancel()
	out, err := h.svc.TokenLease(ctx, principal.From(c), c.Param("name"), &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgLeased, out)
}
