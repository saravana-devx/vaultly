package api

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/saravana/vaultly/internal/audit"
)

// RunJanitor revokes expired dynamic credentials and announces expired secrets.
func (s *Server) RunJanitor(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		s.revokeExpiredLeases(ctx)
		s.announceExpiredSecrets(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) revokeExpiredLeases(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT id FROM dynamic_leases WHERE revoked_at IS NULL AND expires_at < now() ORDER BY expires_at LIMIT 50`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id.String())
		}
	}
	rows.Close()
	for _, id := range ids {
		if err := s.revokeOne(ctx, id, uuid.Nil); err != nil {
			log.Printf("janitor: revoke lease %s: %v", id, err)
		}
	}
}

func (s *Server) announceExpiredSecrets(ctx context.Context) {
	rows, err := s.db.Query(ctx, `UPDATE secrets s SET expiry_notified = true FROM environments e, projects p
		WHERE e.id = s.env_id AND p.id = s.project_id AND s.expires_at < now() AND NOT s.expiry_notified
		RETURNING p.org_id, s.project_id, s.path, s.key, e.slug`)
	if err != nil {
		return
	}
	type ev struct {
		org, project   uuid.UUID
		path, key, env string
	}
	var evs []ev
	for rows.Next() {
		var e ev
		if rows.Scan(&e.org, &e.project, &e.path, &e.key, &e.env) == nil {
			evs = append(evs, e)
		}
	}
	rows.Close()
	for _, e := range evs {
		pid := e.project
		s.audit.Publish(ctx, audit.Event{OrgID: e.org, ProjectID: &pid, ActorType: "system", ActorID: "janitor", ActorLabel: "system",
			Action: "secret.expired", Resource: join(e.path, e.key), Metadata: map[string]any{"env": e.env}})
	}
}
