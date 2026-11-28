package audit

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
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
		FROM audit_logs a LEFT JOIN projects p ON p.id = a.project_id WHERE a.org_id=$1`
	if q.BeforeID != nil {
		// Keyset on (created_at, id), matching the ORDER BY, so events that share the last
		// timestamp of a page are not skipped.
		args = append(args, *q.BeforeID)
		sql += fmt.Sprintf(` AND (a.created_at, a.id) < ($2, $%d)`, len(args))
	} else {
		sql += ` AND a.created_at < $2`
	}
	if q.ProjectID != nil {
		args = append(args, *q.ProjectID)
		sql += fmt.Sprintf(` AND a.project_id = $%d`, len(args))
	}
	if q.Kind != "" {
		args = append(args, q.Kind+".%")
		sql += fmt.Sprintf(` AND a.action LIKE $%d`, len(args))
	}
	args = append(args, q.Limit+1)
	sql += fmt.Sprintf(` ORDER BY a.created_at DESC, a.id DESC LIMIT $%d`, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &Page{Events: []Entry{}}
	for rows.Next() {
		var e Entry
		var md *string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Project, &e.ActorType, &e.Actor, &e.Action, &e.Resource, &md, &e.IP, &e.At); err != nil {
			return nil, err
		}
		e.Metadata = map[string]any{}
		if md != nil {
			_ = r.sealer.OpenJSON(fMetadata, *md, &e.Metadata)
		}
		e.Actor = r.sealer.OpenLabel(e.Actor)
		e.Resource = r.sealer.OpenLabel(e.Resource)
		e.IP = r.sealer.OpenLabel(e.IP)
		if e.Project != nil {
			name := r.sealer.OpenName("projects.name", *e.Project)
			e.Project = &name
		}
		page.Events = append(page.Events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Events) > q.Limit {
		page.Events = page.Events[:q.Limit]
		last := page.Events[q.Limit-1]
		page.Next = last.At.Format(time.RFC3339Nano) + cursorSep + last.ID.String()
	}
	return page, nil
}

// cursorSep joins the timestamp and id of a page's last event in Page.Next.
const cursorSep = "~"

// kindPattern keeps the ?kind= filter to plain action prefixes (letters and underscores only).
var kindPattern = regexp.MustCompile(`^[a-z_]{1,32}$`)

type Handler struct{ reader *Reader }

func NewHandler(reader *Reader) *Handler { return &Handler{reader: reader} }

func (h *Handler) List(c *gin.Context) {
	org, ok := httpx.UUIDParam(c, "org")
	if !ok {
		return
	}
	q := ListQuery{Before: time.Now().Add(time.Minute), Limit: 50}
	ts, idPart, hasID := strings.Cut(c.Query("before"), cursorSep)
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		q.Before = t
		if id, err := uuid.Parse(idPart); hasID && err == nil {
			q.BeforeID = &id
		}
	}
	if id, err := uuid.Parse(c.Query("project_id")); err == nil {
		q.ProjectID = &id
	}
	if k := c.Query("kind"); kindPattern.MatchString(k) {
		q.Kind = k
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
