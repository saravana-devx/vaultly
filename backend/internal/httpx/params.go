package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UUIDParam reads a path parameter. A malformed id is reported as 404, like an unknown one.
func UUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		Error(c, http.StatusNotFound, MsgNotFound)
		return uuid.Nil, false
	}
	return id, true
}

// Timeout bounds a request's work, derived from the client's own context.
func Timeout(c *gin.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), d)
}

const DefaultTimeout = 10 * time.Second
