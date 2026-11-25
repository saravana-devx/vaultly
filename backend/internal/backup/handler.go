package backup

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

const maxUpload = 64 << 20

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Create streams the sealed file; it is a download, not a JSON envelope.
func (h *Handler) Create(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	var body CreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BindError(c, err)
		return
	}
	ctx, cancel := httpx.Timeout(c, 2*time.Minute)
	defer cancel()
	file, err := h.svc.Create(ctx, principal.From(c), org, &body)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, file.Name))
	c.Data(http.StatusOK, "application/octet-stream", file.Data)
}

func (h *Handler) Restore(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUpload)
	upload, _, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Fail(c, ErrNoFile)
		return
	}
	defer upload.Close()
	blob, err := io.ReadAll(upload)
	if err != nil {
		httpx.Fail(c, ErrUnreadable)
		return
	}
	ctx, cancel := httpx.Timeout(c, 5*time.Minute)
	defer cancel()
	res, err := h.svc.Restore(ctx, principal.From(c), org, c.PostForm("passphrase"), blob)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgRestored, res)
}
