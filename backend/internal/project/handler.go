package project

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
	projects, err := h.svc.List(ctx, principal.From(c), org)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgProjectsFetched, projects)
}

func (h *Handler) Create(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	var body CreateProjectRequest
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
	httpx.Success(c, http.StatusCreated, MsgProjectCreated, created)
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	detail, err := h.svc.Get(ctx, principal.From(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgProjectFetched, detail)
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.Delete(ctx, principal.From(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgProjectDeleted, nil)
}

func (h *Handler) CreateEnv(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "pid")
	if !ok {
		return
	}
	var body CreateEnvRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	env, err := h.svc.CreateEnv(ctx, principal.From(c), id, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgEnvCreated, env)
}

func (h *Handler) DeleteEnv(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "pid")
	env, ok2 := httpx.UUIDParam(c, "eid")
	if !ok || !ok2 {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	if err := h.svc.DeleteEnv(ctx, principal.From(c), id, env); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgEnvDeleted, nil)
}
