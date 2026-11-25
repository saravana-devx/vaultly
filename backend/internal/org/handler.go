package org

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	orgs, err := h.svc.List(ctx, principal.From(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgOrgsFetched, orgs)
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateOrgRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	created, err := h.svc.Create(ctx, principal.From(c).UserID, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, MsgOrgCreated, created)
}

func (h *Handler) Stats(c *gin.Context) {
	orgID, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	stats, err := h.svc.Stats(ctx, principal.From(c).UserID, orgID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgStatsFetched, stats)
}

func (h *Handler) EnvSlugs(c *gin.Context) {
	orgID, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	slugs, err := h.svc.EnvSlugs(ctx, principal.From(c).UserID, orgID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgSlugsFetched, slugs)
}
