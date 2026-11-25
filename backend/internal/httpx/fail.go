package httpx

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"vaultly/internal/apperr"
)

var statusByKind = map[apperr.Kind]int{
	apperr.KindInvalid:      http.StatusBadRequest,
	apperr.KindUnauthorized: http.StatusUnauthorized,
	apperr.KindForbidden:    http.StatusForbidden,
	apperr.KindNotFound:     http.StatusNotFound,
	apperr.KindConflict:     http.StatusConflict,
	apperr.KindGone:         http.StatusGone,
	apperr.KindUpstream:     http.StatusBadGateway,
}

// Fail maps a service error to its HTTP response. Anything that is not an apperr.Error is
// logged and reported as a generic 500, so internal details never reach the client.
func Fail(c *gin.Context, err error) {
	status, message := resolve(c, err)
	Error(c, status, message)
}

func AbortFail(c *gin.Context, err error) {
	status, message := resolve(c, err)
	AbortError(c, status, message)
}

func resolve(c *gin.Context, err error) (int, string) {
	if ae, ok := apperr.As(err); ok {
		if status, found := statusByKind[ae.Kind]; found {
			return status, ae.Message
		}
	}
	log.Printf("internal error: %s %s: %v", c.Request.Method, c.FullPath(), err)
	return http.StatusInternalServerError, MsgInternalError
}
