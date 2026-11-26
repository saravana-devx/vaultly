package org

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/pii"
)

type Repository struct {
	db  *pgxpool.Pool
	box *pii.Box
}

func NewRepository(db *pgxpool.Pool, box *pii.Box) *Repository { return &Repository{db: db, box: box} }

func (r *Repository) ListForUser(ctx context.Context, userID uuid.UUID) ([]Org, error) {
	rows, err := r.db.Query(ctx, `SELECT o.id, o.name, m.role FROM orgs o JOIN memberships m ON m.org_id=o.id WHERE m.user_id=$1 ORDER BY o.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Org{}
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Role); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO orgs (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,'admin')`, id, userID)
		return err
	})
	return id, err
}

func (r *Repository) Counts(ctx context.Context, org uuid.UUID) (Counts, error) {
	var c Counts
	queries := []struct {
		sql string
		dst *int
	}{
		{`SELECT count(*) FROM projects WHERE org_id=$1`, &c.Projects},
		{`SELECT count(*) FROM secrets s JOIN projects p ON p.id=s.project_id WHERE p.org_id=$1`, &c.Secrets},
		{`SELECT count(*) FROM service_tokens t JOIN projects p ON p.id=t.project_id WHERE p.org_id=$1`, &c.Tokens},
		{`SELECT count(*) FROM memberships WHERE org_id=$1`, &c.Members},
		{`SELECT count(*) FROM audit_logs WHERE org_id=$1 AND created_at > now() - interval '24 hours'`, &c.Events24},
	}
	for _, q := range queries {
		if err := r.db.QueryRow(ctx, q.sql, org).Scan(q.dst); err != nil {
			return c, err
		}
	}
	return c, nil
}

// Activity returns events per day for the last 14 days, zero-filled.
func (r *Repository) Activity(ctx context.Context, org uuid.UUID) ([]DayCount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_char(d, 'YYYY-MM-DD'), count(a.id) FROM generate_series(current_date - 13, current_date, interval '1 day') d
		LEFT JOIN audit_logs a ON a.org_id=$1 AND a.created_at >= d AND a.created_at < d + interval '1 day'
		GROUP BY d ORDER BY d`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayCount{}
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) Recent(ctx context.Context, org uuid.UUID, limit int) ([]RecentEvent, error) {
	rows, err := r.db.Query(ctx, `SELECT a.actor_label, a.action, a.resource, p.name, a.metadata, a.created_at
		FROM audit_logs a LEFT JOIN projects p ON p.id=a.project_id WHERE a.org_id=$1 ORDER BY a.created_at DESC LIMIT $2`, org, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecentEvent{}
	for rows.Next() {
		var e RecentEvent
		var md *string
		if err := rows.Scan(&e.Actor, &e.Action, &e.Resource, &e.Project, &md, &e.At); err != nil {
			return nil, err
		}
		e.Metadata = map[string]any{}
		if md != nil {
			_ = r.box.OpenJSON("audit.metadata", *md, &e.Metadata)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) PendingInvites(ctx context.Context, org uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM invites WHERE org_id=$1 AND accepted_at IS NULL AND expires_at > now()`, org).Scan(&n)
	return n, err
}

func (r *Repository) TokenExpiries(ctx context.Context, org uuid.UUID) ([]ExpiryRow, error) {
	rows, err := r.db.Query(ctx, `SELECT t.name, p.id, p.name, t.expires_at, t.last_used_at FROM service_tokens t JOIN projects p ON p.id=t.project_id
		WHERE p.org_id=$1 AND ((t.expires_at IS NOT NULL AND t.expires_at < now() + interval '30 days') OR t.last_used_at IS NULL OR t.last_used_at < now() - interval '30 days')
		ORDER BY coalesce(t.expires_at, 'infinity'::timestamptz) LIMIT 6`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExpiryRow{}
	for rows.Next() {
		e := ExpiryRow{Kind: "token"}
		if err := rows.Scan(&e.Name, &e.ProjectID, &e.Project, &e.ExpiresAt, &e.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) SecretExpiries(ctx context.Context, org uuid.UUID) ([]ExpiryRow, error) {
	rows, err := r.db.Query(ctx, `SELECT s.key, p.id, p.name, s.expires_at FROM secrets s JOIN projects p ON p.id=s.project_id
		WHERE p.org_id=$1 AND s.expires_at IS NOT NULL AND s.expires_at < now() + interval '14 days' ORDER BY s.expires_at LIMIT 6`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExpiryRow{}
	for rows.Next() {
		e := ExpiryRow{Kind: "secret"}
		if err := rows.Scan(&e.Name, &e.ProjectID, &e.Project, &e.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) EnvSlugs(ctx context.Context, org uuid.UUID) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT e.slug FROM environments e JOIN projects p ON p.id=e.project_id WHERE p.org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s = r.box.OpenName("environments.slug", s); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out, rows.Err()
}
