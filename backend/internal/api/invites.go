package api

import (
	"crypto/sha256"
	"fmt"
	"html"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const inviteTTL = 7 * 24 * time.Hour

func validEnvRole(r string) bool { return r == "none" || r == "viewer" || r == "developer" }

func (s *Server) envSlugs(c *gin.Context) {
	org, _, ok := s.orgRole(c, "viewer")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT DISTINCT e.slug FROM environments e JOIN projects p ON p.id=e.project_id WHERE p.org_id=$1 ORDER BY e.slug`, org)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil {
			out = append(out, slug)
		}
	}
	c.JSON(200, out)
}

func (s *Server) getMemberEnvRoles(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	uid, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	out := map[string]string{}
	rows, err := s.db.Query(c, `SELECT env_slug, role FROM member_env_roles WHERE org_id=$1 AND user_id=$2`, org, uid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var slug, role string
			if rows.Scan(&slug, &role) == nil {
				out[slug] = role
			}
		}
	}
	c.JSON(200, gin.H{"roles": out})
}

// putMemberEnvRoles replaces a member's overrides. Environments absent from the
// body (or set to "inherit") fall back to the member's workspace role.
func (s *Server) putMemberEnvRoles(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	uid, err := uuid.Parse(c.Param("uid"))
	var in struct {
		Roles map[string]string `json:"roles"`
	}
	if err != nil || c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "invalid request")
		return
	}
	for slug, r := range in.Roles {
		if slugify(slug) != slug || (r != "inherit" && !validEnvRole(r)) {
			fail(c, 400, "roles must be none, viewer, developer or inherit")
			return
		}
	}
	err = pgx.BeginFunc(c, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(c, `DELETE FROM member_env_roles WHERE org_id=$1 AND user_id=$2`, org, uid); err != nil {
			return err
		}
		for slug, r := range in.Roles {
			if r == "inherit" {
				continue
			}
			if _, err := tx.Exec(c, `INSERT INTO member_env_roles (org_id,user_id,env_slug,role) VALUES ($1,$2,$3,$4)`, org, uid, slug, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(c, 404, "member not found")
		return
	}
	s.record(c, org, nil, "member.env_roles", s.emailOf(c, uid), map[string]any{"roles": in.Roles})
	c.Status(204)
}

func (s *Server) createInvite(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	var in struct {
		Email    string
		Role     string
		EnvRoles map[string]string `json:"env_roles"`
	}
	if c.ShouldBindJSON(&in) != nil || !validRole(in.Role) {
		fail(c, 400, "email and a valid role are required")
		return
	}
	in.Email = normEmail(in.Email)
	if _, err := mail.ParseAddress(in.Email); err != nil {
		fail(c, 400, "enter a valid email")
		return
	}
	clean := map[string]string{}
	for slug, r := range in.EnvRoles {
		if r == "inherit" || r == "" {
			continue
		}
		if slugify(slug) != slug || !validEnvRole(r) {
			fail(c, 400, "environment roles must be none, viewer or developer")
			return
		}
		clean[slug] = r
	}
	var already bool
	s.db.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 AND u.email_hash=$2)`, org, s.pii.Index(in.Email)).Scan(&already)
	if already {
		fail(c, 409, "that person is already a member")
		return
	}
	raw, hash := newToken("vin_")
	s.db.Exec(c, `DELETE FROM invites WHERE org_id=$1 AND email_hash=$2 AND accepted_at IS NULL`, org, s.pii.Index(in.Email))
	var id uuid.UUID
	if err := s.db.QueryRow(c, `INSERT INTO invites (org_id,email_enc,email_hash,role,env_roles,token_hash,invited_by,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		org, s.pii.Seal("invites.email", in.Email), s.pii.Index(in.Email), in.Role, clean, hash, s.sealLabel(actorLabel(c)), time.Now().Add(inviteTTL)).Scan(&id); err != nil {
		internalErr(c, err)
		return
	}
	var orgName string
	s.db.QueryRow(c, `SELECT name FROM orgs WHERE id=$1`, org).Scan(&orgName)
	link := s.cfg.AppURL + "/invite/" + raw
	text := fmt.Sprintf("%s invited you to join %s on Vaultly as %s.\n\nAccept the invitation: %s\n\nThis link expires in 7 days.", actorLabel(c), orgName, in.Role, link)
	htmlBody := fmt.Sprintf(`<p><b>%s</b> invited you to join <b>%s</b> on Vaultly as <b>%s</b>.</p><p><a href="%s">Accept the invitation</a></p><p>This link expires in 7 days.</p>`,
		html.EscapeString(actorLabel(c)), html.EscapeString(orgName), html.EscapeString(in.Role), html.EscapeString(link))
	go func() {
		if err := s.mail.Send(in.Email, "You're invited to "+orgName+" on Vaultly", text, htmlBody); err != nil {
			log.Printf("invite email to %s failed: %v", in.Email, err)
		}
	}()
	s.record(c, org, nil, "member.invite", in.Email, map[string]any{"role": in.Role})
	c.JSON(201, gin.H{"id": id, "email": in.Email, "role": in.Role, "link": link})
}

func (s *Server) listInvites(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT id,email_enc,role,env_roles,invited_by,expires_at,created_at FROM invites WHERE org_id=$1 AND accepted_at IS NULL ORDER BY created_at DESC`, org)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var email, role, by string
		var er map[string]string
		var exp, at time.Time
		if rows.Scan(&id, &email, &role, &er, &by, &exp, &at) == nil {
			email, _ = s.pii.Open("invites.email", email)
			by = s.openLabel(by)
			out = append(out, gin.H{"id": id, "email": email, "role": role, "env_roles": er, "invited_by": by, "expires_at": exp, "created_at": at, "expired": exp.Before(time.Now())})
		}
	}
	c.JSON(200, out)
}

func (s *Server) revokeInvite(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	iid, err := uuid.Parse(c.Param("iid"))
	if err != nil {
		fail(c, 404, "not found")
		return
	}
	var email string
	if s.db.QueryRow(c, `DELETE FROM invites WHERE id=$1 AND org_id=$2 RETURNING email_enc`, iid, org).Scan(&email) != nil {
		fail(c, 404, "not found")
		return
	}
	email, _ = s.pii.Open("invites.email", email)
	s.record(c, org, nil, "member.invite_revoke", email, nil)
	c.Status(204)
}

type inviteRow struct {
	ID       uuid.UUID
	OrgID    uuid.UUID
	OrgName  string
	Email    string
	Role     string
	EnvRoles map[string]string
	By       string
	Expires  time.Time
	Accepted *time.Time
}

func (s *Server) loadInvite(c *gin.Context) (*inviteRow, bool) {
	h := sha256.Sum256([]byte(c.Param("token")))
	r := &inviteRow{}
	err := s.db.QueryRow(c, `SELECT i.id,i.org_id,o.name,i.email_enc,i.role,i.env_roles,i.invited_by,i.expires_at,i.accepted_at
		FROM invites i JOIN orgs o ON o.id=i.org_id WHERE i.token_hash=$1`, h[:]).
		Scan(&r.ID, &r.OrgID, &r.OrgName, &r.Email, &r.Role, &r.EnvRoles, &r.By, &r.Expires, &r.Accepted)
	if err != nil {
		fail(c, 404, "this invitation link is invalid")
		return nil, false
	}
	if r.Email, err = s.pii.Open("invites.email", r.Email); err != nil {
		fail(c, 404, "this invitation link is invalid")
		return nil, false
	}
	r.By = s.openLabel(r.By)
	return r, true
}

func maskEmail(e string) string {
	at := strings.Index(e, "@")
	if at < 2 {
		return e
	}
	return e[:1] + strings.Repeat("•", at-1) + e[at:]
}

func (s *Server) previewInvite(c *gin.Context) {
	r, ok := s.loadInvite(c)
	if !ok {
		return
	}
	state := "pending"
	switch {
	case r.Accepted != nil:
		state = "accepted"
	case r.Expires.Before(time.Now()):
		state = "expired"
	}
	hasAccount := s.emailExists(c, r.Email)
	c.JSON(200, gin.H{"org": r.OrgName, "email_hint": maskEmail(r.Email), "role": r.Role, "invited_by": r.By, "state": state, "has_account": hasAccount})
}

func (s *Server) acceptInvite(c *gin.Context) {
	r, ok := s.loadInvite(c)
	if !ok {
		return
	}
	if r.Accepted != nil || r.Expires.Before(time.Now()) {
		fail(c, 410, "this invitation has already been used or has expired")
		return
	}
	if !strings.EqualFold(me(c).Email, r.Email) {
		fail(c, 403, "this invitation was sent to "+maskEmail(r.Email)+" — sign in with that account")
		return
	}
	err := pgx.BeginFunc(c, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(c, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, r.OrgID, me(c).UserID, r.Role); err != nil {
			return err
		}
		for slug, role := range r.EnvRoles {
			if _, err := tx.Exec(c, `INSERT INTO member_env_roles (org_id,user_id,env_slug,role) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, r.OrgID, me(c).UserID, slug, role); err != nil {
				return err
			}
		}
		_, err := tx.Exec(c, `UPDATE invites SET accepted_at=now() WHERE id=$1`, r.ID)
		return err
	})
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, r.OrgID, nil, "member.join", me(c).Email, map[string]any{"role": r.Role})
	c.JSON(200, gin.H{"org_id": r.OrgID, "org": r.OrgName, "role": r.Role})
}
