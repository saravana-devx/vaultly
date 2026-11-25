// Package middleware holds the cross-cutting HTTP concerns: CORS, security headers,
// authentication of users and service tokens, and rate limiting.
package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vaultly/internal/apperr"
	"vaultly/internal/auth"
	"vaultly/internal/httpx"
	"vaultly/internal/principal"
	"vaultly/internal/ratelimit"
	"vaultly/internal/token"
	"vaultly/internal/tokenutil"
)

type KeyFunc func(*gin.Context) string

func ByIP(c *gin.Context) string    { return c.ClientIP() }
func ByUser(c *gin.Context) string  { return principal.From(c).UserID.String() }
func ByToken(c *gin.Context) string { return principal.From(c).Token.ID.String() }

type Set struct {
	sessions *auth.Manager
	tokens   *token.Service
	limiter  ratelimit.Limiter
	origins  map[string]bool
}

func New(sessions *auth.Manager, tokens *token.Service, limiter ratelimit.Limiter, origins []string) *Set {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimSpace(o)] = true
	}
	return &Set{sessions: sessions, tokens: tokens, limiter: limiter, origins: allowed}
}

func (m *Set) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if o := c.GetHeader("Origin"); o != "" && m.origins[o] {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (m *Set) SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

func bearer(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

var errUnauthorized = apperr.Unauthorized("Unauthorized")

// UserAuth accepts a session access token or a personal access token.
func (m *Set) UserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c)
		if strings.HasPrefix(raw, tokenutil.PrefixPersonal) {
			actor, err := m.tokens.AuthenticatePersonal(c.Request.Context(), raw)
			if err != nil {
				httpx.AbortFail(c, err)
				return
			}
			actor.IP = c.ClientIP()
			principal.Set(c, actor)
			c.Next()
			return
		}
		claims, err := m.sessions.ParseAccessToken(raw)
		if err != nil {
			httpx.AbortFail(c, errUnauthorized)
			return
		}
		id, err := parseUUID(claims.Subject)
		if err != nil {
			httpx.AbortFail(c, errUnauthorized)
			return
		}
		principal.Set(c, principal.Actor{UserID: id, Email: claims.Email, IP: c.ClientIP()})
		c.Next()
	}
}

// ServiceAuth accepts only service tokens and enforces their IP allowlist.
func (m *Set) ServiceAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, err := m.tokens.AuthenticateService(c.Request.Context(), bearer(c), c.ClientIP())
		if err != nil {
			httpx.AbortFail(c, err)
			return
		}
		principal.Set(c, actor)
		c.Next()
	}
}

// Limit allows `limit` requests per `window` for each key. If the limiter itself fails it lets
// the request through, so an outage never locks everyone out.
func (m *Set) Limit(name string, limit int, window time.Duration, key KeyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := m.limiter.Allow(c.Request.Context(), "rl:"+name+":"+key(c), limit, window)
		if err != nil {
			c.Next()
			return
		}
		h := c.Writer.Header()
		h.Set("X-RateLimit-Limit", strconv.Itoa(limit))
		h.Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		h.Set("X-RateLimit-Reset", strconv.FormatInt(res.ResetAt.Unix(), 10))
		if !res.Allowed {
			wait := int(time.Until(res.ResetAt).Seconds()) + 1
			h.Set("Retry-After", strconv.Itoa(wait))
			httpx.AbortError(c, http.StatusTooManyRequests, "Too many requests, try again in "+strconv.Itoa(wait)+"s")
			return
		}
		c.Next()
	}
}
