package api

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/saravana/vaultly/internal/audit"
	"github.com/saravana/vaultly/internal/crypto"
	"github.com/saravana/vaultly/internal/webhook"
)

var eventPatternRe = regexp.MustCompile(`^(\*|[a-z_]+\.(\*|[a-z_]+))$`)

func (s *Server) validWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return fmt.Errorf("url must start with http:// or https://")
	}
	if !s.cfg.WebhookAllowPrivate {
		if ip := net.ParseIP(u.Hostname()); ip != nil && webhook.BlockedIP(ip) {
			return fmt.Errorf("private and loopback addresses are not allowed")
		}
		if strings.EqualFold(u.Hostname(), "localhost") {
			return fmt.Errorf("private and loopback addresses are not allowed")
		}
	}
	return nil
}

func normEvents(in []string) ([]string, bool) {
	if len(in) == 0 {
		return []string{"*"}, true
	}
	out := []string{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if !eventPatternRe.MatchString(e) {
			return nil, false
		}
		out = append(out, e)
	}
	return out, len(out) <= 30
}

func (s *Server) listWebhooks(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT w.id, w.name, w.url, w.events, w.active, w.created_at,
		(SELECT status FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY id DESC LIMIT 1),
		(SELECT error FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY id DESC LIMIT 1),
		(SELECT created_at FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY id DESC LIMIT 1)
		FROM webhooks w WHERE w.project_id=$1 ORDER BY w.created_at DESC`, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uuid.UUID
		var name, u string
		var events []string
		var active bool
		var at time.Time
		var status *int
		var lastErr *string
		var lastAt *time.Time
		if rows.Scan(&id, &name, &u, &events, &active, &at, &status, &lastErr, &lastAt) == nil {
			out = append(out, gin.H{"id": id, "name": name, "url": u, "events": events, "active": active, "created_at": at,
				"last_status": status, "last_error": lastErr, "last_delivery_at": lastAt})
		}
	}
	c.JSON(200, out)
}

func (s *Server) createWebhook(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	var in struct {
		Name   string
		URL    string
		Events []string
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "name and url are required")
		return
	}
	if err := s.validWebhookURL(in.URL); err != nil {
		fail(c, 400, err.Error())
		return
	}
	events, eok := normEvents(in.Events)
	if !eok {
		fail(c, 400, "events look like secret.*, token.create or *")
		return
	}
	dek, err := s.dek(c, p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	suffix, _ := randomString(40)
	secret := "whsec_" + suffix
	enc, err := crypto.Encrypt(dek, []byte(secret), []byte(p.ID.String()))
	if err != nil {
		internalErr(c, err)
		return
	}
	var id uuid.UUID
	if err := s.db.QueryRow(c, `INSERT INTO webhooks (project_id,name,url,secret_enc,events) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		p.ID, strings.TrimSpace(in.Name), in.URL, enc, events).Scan(&id); err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, p.OrgID, &p.ID, "webhook.create", in.Name, nil)
	c.JSON(201, gin.H{"id": id, "secret": secret})
}

func (s *Server) loadWebhook(c *gin.Context, p *projectCtx) (webhook.Hook, string, bool) {
	wid, err := uuid.Parse(c.Param("wid"))
	if err != nil {
		fail(c, 404, "not found")
		return webhook.Hook{}, "", false
	}
	h := webhook.Hook{ID: wid, ProjectID: p.ID}
	var enc []byte
	var name string
	if s.db.QueryRow(c, `SELECT url, secret_enc, events, name FROM webhooks WHERE id=$1 AND project_id=$2`, wid, p.ID).Scan(&h.URL, &enc, &h.Events, &name) != nil {
		fail(c, 404, "not found")
		return h, "", false
	}
	dek, err := s.dek(c, p.ID)
	if err == nil {
		var sec []byte
		if sec, err = crypto.Decrypt(dek, enc, []byte(p.ID.String())); err == nil {
			h.Secret = string(sec)
		}
	}
	if err != nil {
		internalErr(c, err)
		return h, "", false
	}
	return h, name, true
}

func (s *Server) updateWebhook(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	_, name, ok := s.loadWebhook(c, p)
	if !ok {
		return
	}
	var in struct {
		Name   *string
		URL    *string
		Events *[]string
		Active *bool
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "invalid request")
		return
	}
	wid := uuid.MustParse(c.Param("wid"))
	if in.URL != nil {
		if err := s.validWebhookURL(*in.URL); err != nil {
			fail(c, 400, err.Error())
			return
		}
		s.db.Exec(c, `UPDATE webhooks SET url=$2 WHERE id=$1`, wid, *in.URL)
	}
	if in.Events != nil {
		ev, eok := normEvents(*in.Events)
		if !eok {
			fail(c, 400, "invalid event pattern")
			return
		}
		s.db.Exec(c, `UPDATE webhooks SET events=$2 WHERE id=$1`, wid, ev)
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		s.db.Exec(c, `UPDATE webhooks SET name=$2 WHERE id=$1`, wid, strings.TrimSpace(*in.Name))
		name = *in.Name
	}
	if in.Active != nil {
		s.db.Exec(c, `UPDATE webhooks SET active=$2 WHERE id=$1`, wid, *in.Active)
	}
	s.record(c, p.OrgID, &p.ID, "webhook.update", name, nil)
	c.Status(204)
}

func (s *Server) deleteWebhook(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	_, name, ok := s.loadWebhook(c, p)
	if !ok {
		return
	}
	s.db.Exec(c, `DELETE FROM webhooks WHERE id=$1`, c.Param("wid"))
	s.record(c, p.OrgID, &p.ID, "webhook.delete", name, nil)
	c.Status(204)
}

func (s *Server) testWebhook(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	h, _, ok := s.loadWebhook(c, p)
	if !ok {
		return
	}
	ev := audit.Event{ID: uuid.New(), OrgID: p.OrgID, ProjectID: &p.ID, ActorType: "user", ActorLabel: me(c).Email, Action: "webhook.test", Resource: "ping", At: time.Now().UTC()}
	res := s.hooks.Deliver(c, h, ev, p.Name, false)
	s.hooks.Record(c, h.ID, ev, res)
	c.JSON(200, gin.H{"status": res.Status, "error": res.Error, "duration_ms": res.Duration.Milliseconds()})
}

func (s *Server) webhookDeliveries(c *gin.Context) {
	p, ok := s.project(c, "admin")
	if !ok {
		return
	}
	if _, _, ok := s.loadWebhook(c, p); !ok {
		return
	}
	rows, err := s.db.Query(c, `SELECT action,status,error,attempts,duration_ms,created_at FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY id DESC LIMIT 30`, c.Param("wid"))
	if err != nil {
		internalErr(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var action, e string
		var status, attempts, ms int
		var at time.Time
		if rows.Scan(&action, &status, &e, &attempts, &ms, &at) == nil {
			out = append(out, gin.H{"action": action, "status": status, "error": e, "attempts": attempts, "duration_ms": ms, "at": at})
		}
	}
	c.JSON(200, out)
}
