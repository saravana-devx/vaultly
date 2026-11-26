package webhook

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/audit"
	"vaultly/internal/dberr"
	"vaultly/internal/pii"
)

// Webhook names, URLs, event lists and delivery details are stored sealed.
const (
	fName    = "webhooks.name"
	fURL     = "webhooks.url"
	fEvents  = "webhooks.events"
	fAction  = "webhook_deliveries.action"
	fError   = "webhook_deliveries.error"
	fProject = "projects.name"
)

type Repository struct {
	db  *pgxpool.Pool
	box *pii.Box
}

func NewRepository(db *pgxpool.Pool, box *pii.Box) *Repository { return &Repository{db: db, box: box} }

func (r *Repository) events(raw string) []string {
	out := []string{}
	if err := r.box.OpenJSON(fEvents, raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func (r *Repository) List(ctx context.Context, projectID uuid.UUID) ([]View, error) {
	rows, err := r.db.Query(ctx, `SELECT w.id, w.name, w.url, w.events, w.active, w.created_at,
		(SELECT status FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY id DESC LIMIT 1),
		(SELECT error FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY id DESC LIMIT 1),
		(SELECT created_at FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY id DESC LIMIT 1)
		FROM webhooks w WHERE w.project_id=$1 ORDER BY w.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []View{}
	for rows.Next() {
		var v View
		var events string
		if err := rows.Scan(&v.ID, &v.Name, &v.URL, &events, &v.Active, &v.CreatedAt, &v.LastStatus, &v.LastError, &v.LastDeliveryAt); err != nil {
			return nil, err
		}
		v.Name, v.URL, v.Events = r.box.OpenName(fName, v.Name), r.box.OpenName(fURL, v.URL), r.events(events)
		if v.LastError != nil {
			msg := r.box.OpenName(fError, *v.LastError)
			v.LastError = &msg
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, projectID uuid.UUID, name, url string, secretEnc []byte, events []string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `INSERT INTO webhooks (project_id,name,url,secret_enc,events) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		projectID, r.box.SealName(fName, name), r.box.SealName(fURL, url), secretEnc, r.box.SealJSON(fEvents, events)).Scan(&id)
	return id, err
}

func (r *Repository) Get(ctx context.Context, projectID, id uuid.UUID) (*HookRow, error) {
	h := &HookRow{ID: id}
	var events string
	err := r.db.QueryRow(ctx, `SELECT url, secret_enc, events, name FROM webhooks WHERE id=$1 AND project_id=$2`, id, projectID).
		Scan(&h.URL, &h.SecretEnc, &events, &h.Name)
	if dberr.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	h.URL, h.Name, h.Events = r.box.OpenName(fURL, h.URL), r.box.OpenName(fName, h.Name), r.events(events)
	return h, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, u Update) error {
	if u.URL != nil {
		if _, err := r.db.Exec(ctx, `UPDATE webhooks SET url=$2 WHERE id=$1`, id, r.box.SealName(fURL, *u.URL)); err != nil {
			return err
		}
	}
	if u.Events != nil {
		if _, err := r.db.Exec(ctx, `UPDATE webhooks SET events=$2 WHERE id=$1`, id, r.box.SealJSON(fEvents, *u.Events)); err != nil {
			return err
		}
	}
	if u.Name != nil {
		if _, err := r.db.Exec(ctx, `UPDATE webhooks SET name=$2 WHERE id=$1`, id, r.box.SealName(fName, *u.Name)); err != nil {
			return err
		}
	}
	if u.Active != nil {
		if _, err := r.db.Exec(ctx, `UPDATE webhooks SET active=$2 WHERE id=$1`, id, *u.Active); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
	return err
}

func (r *Repository) Deliveries(ctx context.Context, id uuid.UUID) ([]Delivery, error) {
	rows, err := r.db.Query(ctx, `SELECT action,status,error,attempts,duration_ms,created_at FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY id DESC LIMIT 30`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.Action, &d.Status, &d.Error, &d.Attempts, &d.DurationMS, &d.At); err != nil {
			return nil, err
		}
		d.Action, d.Error = r.box.OpenName(fAction, d.Action), r.box.OpenName(fError, d.Error)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ActiveHooks implements Store for the dispatcher.
func (r *Repository) ActiveHooks(ctx context.Context, projectID uuid.UUID) ([]HookRow, string, error) {
	var name string
	if err := r.db.QueryRow(ctx, `SELECT name FROM projects WHERE id=$1`, projectID).Scan(&name); err != nil {
		return nil, "", err
	}
	name = r.box.OpenName(fProject, name)
	rows, err := r.db.Query(ctx, `SELECT id, url, secret_enc, events FROM webhooks WHERE project_id=$1 AND active`, projectID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []HookRow
	for rows.Next() {
		var h HookRow
		var events string
		if err := rows.Scan(&h.ID, &h.URL, &h.SecretEnc, &events); err != nil {
			continue
		}
		h.URL, h.Events = r.box.OpenName(fURL, h.URL), r.events(events)
		out = append(out, h)
	}
	return out, name, rows.Err()
}

// RecordDelivery stores the outcome and keeps only the latest 100 per webhook.
func (r *Repository) RecordDelivery(ctx context.Context, hookID uuid.UUID, ev audit.Event, res Result) {
	if _, err := r.db.Exec(ctx, `INSERT INTO webhook_deliveries (webhook_id,event_id,action,status,error,attempts,duration_ms) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		hookID, ev.ID, r.box.SealName(fAction, ev.Action), res.Status, r.box.SealName(fError, res.Error), res.Attempts, res.Duration.Milliseconds()); err != nil {
		log.Printf("webhook: record delivery: %v", err)
	}
	_, _ = r.db.Exec(ctx, `DELETE FROM webhook_deliveries WHERE webhook_id=$1 AND id NOT IN (SELECT id FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY id DESC LIMIT 100)`, hookID)
}
