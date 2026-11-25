package audit

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/access"
	"vaultly/internal/httpx"
	"vaultly/internal/principal"
)

type Reader struct {
	db     *pgxpool.Pool
	access *access.Service
	sealer Sealer
}

func NewReader(db *pgxpool.Pool, access *access.Service, sealer Sealer) *Reader {
	return &Reader{db: db, access: access, sealer: sealer}
}

// List returns one page of the workspace's activity, newest first. Only admins may read it.
func (r *Reader) List(ctx context.Context, a principal.Actor, org uuid.UUID, q ListQuery) (*Page, error) {
	if _, err := r.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	args := []any{org, q.Before}
	sql := `SELECT a.id, a.project_id, p.name, a.actor_type, a.actor_label, a.action, a.resource, a.metadata, a.ip, a.created_at
		FROM audit_logs a LEFT JOIN projects p ON p.id = a.project_id WHERE a.org_id=$1 AND a.created_at < $2`
	if q.ProjectID != nil {
		args = append(args, *q.ProjectID)
		sql += ` AND a.project_id = $3`
	}
	args = append(args, q.Limit+1)
	sql += ` ORDER BY a.created_at DESC LIMIT $` + string(rune('0'+len(args)))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &Page{Events: []Entry{}}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Project, &e.ActorType, &e.Actor, &e.Action, &e.Resource, &e.Metadata, &e.IP, &e.At); err != nil {
			return nil, err
		}
		e.Actor = r.sealer.OpenLabel(e.Actor)
		e.Resource = r.sealer.OpenLabel(e.Resource)
		page.Events = append(page.Events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Events) > q.Limit {
		page.Events = page.Events[:q.Limit]
		page.Next = page.Events[q.Limit-1].At.Format(time.RFC3339Nano)
	}
	return page, nil
}

type Handler struct{ reader *Reader }

func NewHandler(reader *Reader) *Handler { return &Handler{reader: reader} }

func (h *Handler) List(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	q := ListQuery{Before: time.Now().Add(time.Minute), Limit: 50}
	if t, err := time.Parse(time.RFC3339Nano, c.Query("before")); err == nil {
		q.Before = t
	}
	if id, err := uuid.Parse(c.Query("project_id")); err == nil {
		q.ProjectID = &id
	}
	ctx, cancel := httpx.Timeout(c, httpx.DefaultTimeout)
	defer cancel()
	page, err := h.reader.List(ctx, principal.From(c), org, q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, MsgFetched, page)
}
