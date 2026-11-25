package secret

import (
	"net/http"
	"strings"

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
	out, err := h.svc.List(ctx, principal.From(c), pid, Query{Env: c.Query("env"), Path: c.Query("path"), Tag: c.Query("tag")})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSecretsFetched, out)
}

func (h *Handler) Search(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Search(ctx, principal.From(c), pid, c.Query("env"), c.Query("q"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSearchDone, out)
}

func (h *Handler) Get(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	sid, ok2 := httpx.UUIDParam(c, "sid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Get(ctx, principal.From(c), pid, sid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSecretFetched, out)
}

func (h *Handler) Put(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	var body PutRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Put(ctx, principal.From(c), pid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSecretSaved, out)
}

func (h *Handler) Delete(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	sid, ok2 := httpx.UUIDParam(c, "sid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.Delete(ctx, principal.From(c), pid, sid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSecretDeleted, nil)
}

func (h *Handler) Versions(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	sid, ok2 := httpx.UUIDParam(c, "sid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Versions(ctx, principal.From(c), pid, sid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgVersionsFetched, out)
}

func (h *Handler) Rollback(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	sid, ok2 := httpx.UUIDParam(c, "sid")
	if !ok || !ok2 {
		return
	}
	var body RollbackRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Rollback(ctx, principal.From(c), pid, sid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgRolledBack, out)
}

func (h *Handler) Rotate(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	sid, ok2 := httpx.UUIDParam(c, "sid")
	if !ok || !ok2 {
		return
	}
	var body RotateRequest
	_ = c.ShouldBindJSON(&body)
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Rotate(ctx, principal.From(c), pid, sid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgRotated, out)
}

func (h *Handler) Import(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	var body ImportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.Import(ctx, principal.From(c), pid, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgImported, out)
}

func (h *Handler) Export(c *gin.Context) {
	pid, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	pairs, err := h.svc.Export(ctx, principal.From(c), pid, ExportQuery{
		Env: c.Query("env"), Path: c.Query("path"), Recursive: c.Query("recursive") == "true", Resolve: c.Query("resolve") != "false",
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	respond(c, pairs)
}

// Fetch serves the machine API (service token).
func (h *Handler) Fetch(c *gin.Context) {
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	pairs, err := h.svc.Fetch(ctx, principal.From(c), c.Query("path"), c.Query("recursive") == "true", c.Query("resolve") != "false")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	respond(c, pairs)
}

func (h *Handler) MachinePut(c *gin.Context) {
	var body MachinePutRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	out, err := h.svc.MachinePut(ctx, principal.From(c), &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSecretSaved, out)
}

// respond writes dotenv/shell as plain text (consumed by shells and CI) and json inside the envelope.
func respond(c *gin.Context, pairs []Pair) {
	switch format := strings.ToLower(c.DefaultQuery("format", "json")); format {
	case "dotenv", "shell":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(RenderText(pairs, format)))
	default:
		httpx.Success(c, http.StatusOK, MsgExported, gin.H{"secrets": ToMap(pairs)})
	}
}
