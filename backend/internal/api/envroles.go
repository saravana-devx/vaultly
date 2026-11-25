package api

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var envRank = map[string]int{"none": 0, "viewer": 1, "developer": 2, "admin": 3}

// envRole resolves a member's effective role in one environment: an explicit
// per-environment override wins, otherwise the workspace role applies.
func (s *Server) envRole(ctx context.Context, p *projectCtx, userID uuid.UUID, slug string) string {
	if p.Role == "admin" {
		return "admin"
	}
	var r string
	if err := s.db.QueryRow(ctx, `SELECT role FROM member_env_roles WHERE org_id=$1 AND user_id=$2 AND env_slug=$3`,
		p.OrgID, userID, slug).Scan(&r); err == nil {
		return r
	}
	return p.Role
}

func (s *Server) needEnv(c *gin.Context, p *projectCtx, slug, need string) bool {
	if envRank[s.envRole(c, p, me(c).UserID, slug)] >= envRank[need] {
		return true
	}
	fail(c, 403, fmt.Sprintf("you need %s access to the %s environment", need, slug))
	return false
}

// readableEnvs returns the slugs of environments the caller may read.
func (s *Server) readableEnvs(c *gin.Context, p *projectCtx) map[string]uuid.UUID {
	out := map[string]uuid.UUID{}
	for _, e := range s.envs(c, p.ID) {
		if envRank[s.envRole(c, p, me(c).UserID, e.Slug)] >= envRank["viewer"] {
			out[e.Slug] = e.ID
		}
	}
	return out
}
