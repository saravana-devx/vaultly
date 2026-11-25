package api

import (
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type keyFunc func(*gin.Context) string

func byIP(c *gin.Context) string { return c.ClientIP() }

func byUser(c *gin.Context) string { return me(c).UserID.String() }

func byToken(c *gin.Context) string { return me(c).Token.ID.String() }

func (s *Server) rateLimit(name string, limit int, window time.Duration, key keyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := s.limiter.Allow(c, "rl:"+name+":"+key(c), limit, window)
		if err != nil {
			log.Printf("ratelimit %s: %v (failing open)", name, err)
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
			fail(c, 429, "too many requests, try again in "+strconv.Itoa(wait)+"s")
			return
		}
		c.Next()
	}
}
