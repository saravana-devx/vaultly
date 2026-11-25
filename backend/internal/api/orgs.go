package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/saravana/vaultly/internal/audit"
)

func auditEvent(orgID uuid.UUID, u userDTO, action, ip string) audit.Event {
	return audit.Event{OrgID: orgID, ActorType: "user", ActorID: u.ID.String(), ActorLabel: u.Email, Action: action, IP: ip}
}

func (s *Server) listOrgs(c *gin.Context) {
	rows, err := s.db.Query(c, `SELECT o.id, o.name, m.role FROM orgs o JOIN memberships m ON m.org_id=o.id
		WHERE m.user_id=$1 ORDER BY o.created_at`, me(c).UserID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var name, role string
		if rows.Scan(&id, &name, &role) == nil {
			out = append(out, gin.H{"id": id, "name": name, "role": role})
		}
	}
	c.JSON(200, out)
}

func (s *Server) createOrg(c *gin.Context) {
	var in struct{ Name string }
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "name is required")
		return
	}
	var id uuid.UUID
	err := pgx.BeginFunc(c, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO orgs (name) VALUES ($1) RETURNING id`, strings.TrimSpace(in.Name)).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(c, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,'admin')`, id, me(c).UserID)
		return err
	})
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id, "name": strings.TrimSpace(in.Name), "role": "admin"})
}

func (s *Server) orgStats(c *gin.Context) {
	org, role, ok := s.orgRole(c, "viewer")
	if !ok {
		return
	}
	var projects, secrets, tokens, members, events int
	s.db.QueryRow(c, `SELECT count(*) FROM projects WHERE org_id=$1`, org).Scan(&projects)
	s.db.QueryRow(c, `SELECT count(*) FROM secrets s JOIN projects p ON p.id=s.project_id WHERE p.org_id=$1`, org).Scan(&secrets)
	s.db.QueryRow(c, `SELECT count(*) FROM service_tokens t JOIN projects p ON p.id=t.project_id WHERE p.org_id=$1`, org).Scan(&tokens)
	s.db.QueryRow(c, `SELECT count(*) FROM memberships WHERE org_id=$1`, org).Scan(&members)
	s.db.QueryRow(c, `SELECT count(*) FROM audit_logs WHERE org_id=$1 AND created_at > now() - interval '24 hours'`, org).Scan(&events)

	out := gin.H{"projects": projects, "secrets": secrets, "tokens": tokens, "members": members, "events_24h": events}

	// events per day for the last 14 days (zero-filled)
	activity := []gin.H{}
	if rows, err := s.db.Query(c, `
		SELECT to_char(d, 'YYYY-MM-DD'), count(a.id) FROM generate_series(current_date - 13, current_date, interval '1 day') d
		LEFT JOIN audit_logs a ON a.org_id=$1 AND a.created_at >= d AND a.created_at < d + interval '1 day'
		GROUP BY d ORDER BY d`, org); err == nil {
		for rows.Next() {
			var day string
			var n int
			if rows.Scan(&day, &n) == nil {
				activity = append(activity, gin.H{"day": day, "count": n})
			}
		}
		rows.Close()
	}
	out["activity"] = activity

	if role == "admin" { // audit data and token inventory are admin-only
		recent := []gin.H{}
		if rows, err := s.db.Query(c, `SELECT a.actor_label, a.action, a.resource, p.name, a.metadata, a.created_at
			FROM audit_logs a LEFT JOIN projects p ON p.id=a.project_id WHERE a.org_id=$1 ORDER BY a.created_at DESC LIMIT 8`, org); err == nil {
			for rows.Next() {
				var actor, action, resource string
				var pname *string
				var md map[string]any
				var at time.Time
				if rows.Scan(&actor, &action, &resource, &pname, &md, &at) == nil {
					recent = append(recent, gin.H{"actor": s.openLabel(actor), "action": action, "resource": s.openLabel(resource), "project": pname, "metadata": md, "at": at})
				}
			}
			rows.Close()
		}
		out["recent"] = recent
		var pending int
		s.db.QueryRow(c, `SELECT count(*) FROM invites WHERE org_id=$1 AND accepted_at IS NULL AND expires_at > now()`, org).Scan(&pending)
		out["pending_invites"] = pending

		attention := []gin.H{}
		if rows, err := s.db.Query(c, `SELECT t.name, p.id, p.name, t.expires_at, t.last_used_at FROM service_tokens t JOIN projects p ON p.id=t.project_id
			WHERE p.org_id=$1 AND ((t.expires_at IS NOT NULL AND t.expires_at < now() + interval '30 days') OR t.last_used_at IS NULL OR t.last_used_at < now() - interval '30 days')
			ORDER BY coalesce(t.expires_at, 'infinity'::timestamptz) LIMIT 6`, org); err == nil {
			for rows.Next() {
				var name, pname string
				var pid uuid.UUID
				var exp, used *time.Time
				if rows.Scan(&name, &pid, &pname, &exp, &used) != nil {
					continue
				}
				reason := "Not used in 30+ days"
				if exp != nil && exp.Before(time.Now()) {
					reason = "Expired"
				} else if exp != nil && exp.Before(time.Now().Add(30*24*time.Hour)) {
					reason = "Expires in " + itoa(int(time.Until(*exp).Hours()/24)+1) + " days"
				}
				attention = append(attention, gin.H{"kind": "token", "token": name, "project_id": pid, "project": pname, "reason": reason})
			}
			rows.Close()
		}
		if rows, err := s.db.Query(c, `SELECT s.key, p.id, p.name, s.expires_at FROM secrets s JOIN projects p ON p.id=s.project_id
			WHERE p.org_id=$1 AND s.expires_at IS NOT NULL AND s.expires_at < now() + interval '14 days' ORDER BY s.expires_at LIMIT 6`, org); err == nil {
			for rows.Next() {
				var key, pname string
				var pid uuid.UUID
				var exp time.Time
				if rows.Scan(&key, &pid, &pname, &exp) != nil {
					continue
				}
				reason := "Secret expired"
				if exp.After(time.Now()) {
					reason = "Secret expires in " + itoa(int(time.Until(exp).Hours()/24)+1) + " days"
				}
				attention = append(attention, gin.H{"kind": "secret", "token": key, "project_id": pid, "project": pname, "reason": reason})
			}
			rows.Close()
		}
		out["attention"] = attention
	}
	c.JSON(200, out)
}

func itoa(n int) string { return strconv.Itoa(n) }

// ---- members ----

func (s *Server) listMembers(c *gin.Context) {
	org, _, ok := s.orgRole(c, "viewer")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT u.id,u.email_enc,u.name_enc,m.role,m.created_at FROM memberships m
		JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 ORDER BY m.created_at`, org)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	type member struct {
		id                uuid.UUID
		email, name, role string
		at                time.Time
	}
	var ms []member
	for rows.Next() {
		var m member
		var emailEnc, nameEnc string
		if rows.Scan(&m.id, &emailEnc, &nameEnc, &m.role, &m.at) == nil {
			if u, err := s.openUser(m.id, emailEnc, nameEnc); err == nil {
				m.email, m.name = u.Email, u.Name
				ms = append(ms, m)
			}
		}
	}
	rows.Close()
	overrides := map[uuid.UUID]map[string]string{}
	if rr, err := s.db.Query(c, `SELECT user_id, env_slug, role FROM member_env_roles WHERE org_id=$1`, org); err == nil {
		for rr.Next() {
			var uid uuid.UUID
			var slug, role string
			if rr.Scan(&uid, &slug, &role) == nil {
				if overrides[uid] == nil {
					overrides[uid] = map[string]string{}
				}
				overrides[uid][slug] = role
			}
		}
		rr.Close()
	}
	out := []gin.H{}
	for _, m := range ms {
		er := overrides[m.id]
		if er == nil {
			er = map[string]string{}
		}
		out = append(out, gin.H{"id": m.id, "email": m.email, "name": m.name, "role": m.role, "joined_at": m.at, "env_roles": er})
	}
	c.JSON(200, out)
}

func (s *Server) addMember(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	var in struct{ Email, Role string }
	if c.ShouldBindJSON(&in) != nil || !validRole(in.Role) {
		fail(c, 400, "email and a valid role are required")
		return
	}
	uid, err := s.userIDByEmail(c, in.Email)
	if err != nil {
		fail(c, 404, "no user with that email — ask them to sign up first")
		return
	}
	if _, err := s.db.Exec(c, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,$3)`, org, uid, in.Role); err != nil {
		if isUnique(err) {
			fail(c, 409, "already a member")
			return
		}
		internalErr(c, err)
		return
	}
	s.record(c, org, nil, "member.add", in.Email, map[string]any{"role": in.Role})
	c.Status(201)
}

// lastAdmin reports whether uid is the only admin of org.
func (s *Server) lastAdmin(c *gin.Context, org, uid uuid.UUID) bool {
	var n int
	var isAdmin bool
	s.db.QueryRow(c, `SELECT count(*), bool_or(user_id=$2) FROM memberships WHERE org_id=$1 AND role='admin'`, org, uid).Scan(&n, &isAdmin)
	return isAdmin && n <= 1
}

func (s *Server) updateMember(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	uid, err := uuid.Parse(c.Param("uid"))
	var in struct{ Role string }
	if err != nil || c.ShouldBindJSON(&in) != nil || !validRole(in.Role) {
		fail(c, 400, "invalid request")
		return
	}
	if in.Role != "admin" && s.lastAdmin(c, org, uid) {
		fail(c, 409, "a workspace needs at least one admin")
		return
	}
	tag, err := s.db.Exec(c, `UPDATE memberships SET role=$3 WHERE org_id=$1 AND user_id=$2`, org, uid, in.Role)
	if err != nil || tag.RowsAffected() == 0 {
		fail(c, 404, "not found")
		return
	}
	s.record(c, org, nil, "member.role_change", s.emailOf(c, uid), map[string]any{"role": in.Role})
	c.Status(204)
}

func (s *Server) removeMember(c *gin.Context) {
	org, _, ok := s.orgRole(c, "viewer")
	if !ok {
		return
	}
	uid, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		fail(c, 400, "invalid request")
		return
	}
	// admins can remove anyone; anyone can leave
	if uid != me(c).UserID {
		if _, _, ok := s.orgRole(c, "admin"); !ok {
			return
		}
	}
	if s.lastAdmin(c, org, uid) {
		fail(c, 409, "a workspace needs at least one admin")
		return
	}
	s.db.Exec(c, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, org, uid)
	s.record(c, org, nil, "member.remove", s.emailOf(c, uid), nil)
	c.Status(204)
}

// ---- projects & environments ----

type envDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

func (s *Server) envs(c *gin.Context, pid uuid.UUID) []envDTO {
	out := []envDTO{}
	rows, err := s.db.Query(c, `SELECT id,name,slug FROM environments WHERE project_id=$1 ORDER BY position, name`, pid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var e envDTO
		if rows.Scan(&e.ID, &e.Name, &e.Slug) == nil {
			out = append(out, e)
		}
	}
	return out
}

func (s *Server) listProjects(c *gin.Context) {
	org, _, ok := s.orgRole(c, "viewer")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT p.id,p.name,p.created_at,
		(SELECT count(*) FROM secrets WHERE project_id=p.id),
		(SELECT max(created_at) FROM audit_logs WHERE project_id=p.id) FROM projects p WHERE p.org_id=$1 ORDER BY p.name`, org)
	if err != nil {
		internalErr(c, err)
		return
	}
	type row struct {
		id      uuid.UUID
		name    string
		at      time.Time
		secrets int
		last    *time.Time
	}
	var rs []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.name, &r.at, &r.secrets, &r.last) == nil {
			rs = append(rs, r)
		}
	}
	rows.Close()
	out := []gin.H{}
	for _, r := range rs {
		out = append(out, gin.H{"id": r.id, "name": r.name, "created_at": r.at, "secret_count": r.secrets, "last_activity": r.last, "environments": s.envs(c, r.id)})
	}
	c.JSON(200, out)
}

func (s *Server) createProject(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	var in struct{ Name string }
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" || len(in.Name) > 80 {
		fail(c, 400, "project name is required")
		return
	}
	_, wrapped, err := s.keys.NewDEK(c)
	if err != nil {
		internalErr(c, err)
		return
	}
	var pid uuid.UUID
	err = pgx.BeginFunc(c, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO projects (org_id,name,encrypted_dek) VALUES ($1,$2,$3) RETURNING id`,
			org, strings.TrimSpace(in.Name), wrapped).Scan(&pid); err != nil {
			return err
		}
		for i, e := range [][2]string{{"Development", "dev"}, {"Staging", "staging"}, {"Production", "prod"}} {
			if _, err := tx.Exec(c, `INSERT INTO environments (project_id,name,slug,position) VALUES ($1,$2,$3,$4)`, pid, e[0], e[1], i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, org, &pid, "project.create", in.Name, nil)
	c.JSON(201, gin.H{"id": pid, "name": strings.TrimSpace(in.Name), "environments": s.envs(c, pid)})
}

func (s *Server) getProject(c *gin.Context) {
	p, ok := s.project(c, "viewer")
	if !ok {
		return
	}
	envs := s.envs(c, p.ID)
	if p.Role != "admin" {
		visible := envs[:0]
		for _, e := range envs {
			if envRank[s.envRole(c, p, me(c).UserID, e.Slug)] >= envRank["viewer"] {
				visible = append(visible, e)
			}
		}
		envs = visible
	}
	c.JSON(200, gin.H{"id": p.ID, "org_id": p.OrgID, "name": p.Name, "role": p.Role, "environments": envs})
}

func (s *Server) deleteProject(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	s.revokeLeasesWhere(c, `d.project_id=$1`, p.ID)
	s.db.Exec(c, `DELETE FROM projects WHERE id=$1`, p.ID)
	s.deks.forget(p.ID)
	s.record(c, p.OrgID, &p.ID, "project.delete", p.Name, nil)
	c.Status(204)
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *Server) createEnv(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var in struct{ Name, Slug string }
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "name is required")
		return
	}
	slug := slugify(in.Slug)
	if slug == "" {
		slug = slugify(in.Name)
	}
	if slug == "" || len(slug) > 32 {
		fail(c, 400, "invalid environment slug")
		return
	}
	var e envDTO
	err := s.db.QueryRow(c, `INSERT INTO environments (project_id,name,slug,position)
		VALUES ($1,$2,$3,(SELECT coalesce(max(position),0)+1 FROM environments WHERE project_id=$1)) RETURNING id,name,slug`,
		p.ID, strings.TrimSpace(in.Name), slug).Scan(&e.ID, &e.Name, &e.Slug)
	if isUnique(err) {
		fail(c, 409, "an environment with that slug already exists")
		return
	} else if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "environment.create", e.Slug, nil)
	c.JSON(201, e)
}

func (s *Server) deleteEnv(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	eid, err := uuid.Parse(c.Param("eid"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	s.revokeLeasesWhere(c, `d.env_id=$1`, eid)
	var slug string
	if s.db.QueryRow(c, `DELETE FROM environments WHERE id=$1 AND project_id=$2 RETURNING slug`, eid, p.ID).Scan(&slug) != nil {
		fail(c, 404, "not found")
		return
	}
	s.record(c, p.OrgID, &p.ID, "environment.delete", slug, nil)
	c.Status(204)
}

// ---- audit ----

func (s *Server) listAudit(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	limit := 50
	before := time.Now().Add(time.Minute)
	if t, err := time.Parse(time.RFC3339Nano, c.Query("before")); err == nil {
		before = t
	}
	args := []any{org, before}
	q := `SELECT a.id, a.project_id, p.name, a.actor_type, a.actor_label, a.action, a.resource, a.metadata, a.ip, a.created_at
		FROM audit_logs a LEFT JOIN projects p ON p.id = a.project_id WHERE a.org_id=$1 AND a.created_at < $2`
	if pid := c.Query("project_id"); pid != "" {
		if id, err := uuid.Parse(pid); err == nil {
			args = append(args, id)
			q += ` AND a.project_id = $3`
		}
	}
	q += ` ORDER BY a.created_at DESC LIMIT ` + "51"
	rows, err := s.db.Query(c, q, args...)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var pid *uuid.UUID
		var pname *string
		var at, label, action, resource, ip string
		var md map[string]any
		var ts time.Time
		if rows.Scan(&id, &pid, &pname, &at, &label, &action, &resource, &md, &ip, &ts) == nil {
			out = append(out, gin.H{"id": id, "project_id": pid, "project": pname, "actor_type": at, "actor": s.openLabel(label),
				"action": action, "resource": s.openLabel(resource), "metadata": md, "ip": ip, "at": ts})
		}
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1]["at"].(time.Time).Format(time.RFC3339Nano)
	}
	c.JSON(200, gin.H{"events": out, "next": next})
}

// emailOf names a user in audit events; it falls back to the ID if the user is gone.
func (s *Server) emailOf(c *gin.Context, uid uuid.UUID) string {
	u, err := s.loadUser(c, uid)
	if err != nil {
		return uid.String()
	}
	return u.Email
}
