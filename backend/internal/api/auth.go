package api

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/saravana/vaultly/internal/auth"
)

const refreshCookie = "vaultly_rt"

type userDTO struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

func (s *Server) setRefreshCookie(c *gin.Context, tok string, maxAge int) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	http.SetCookie(c.Writer, &http.Cookie{
		Name: refreshCookie, Value: tok, Path: "/api/auth", MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) issue(c *gin.Context, u userDTO) {
	access, err := s.auth.NewAccessToken(u.ID, u.Email)
	if err != nil {
		internalErr(c, err)
		return
	}
	rt, err := s.auth.NewRefreshToken(c, u.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	s.setRefreshCookie(c, rt, int(auth.RefreshTTL.Seconds()))
	c.JSON(200, gin.H{"access_token": access, "expires_in": int(auth.AccessTTL.Seconds()), "user": u})
}

func (s *Server) register(c *gin.Context) {
	var in struct{ Email, Password, Name, OrgName string }
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, 400, "invalid request")
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if _, err := mail.ParseAddress(in.Email); err != nil {
		fail(c, 400, "enter a valid email")
		return
	}
	if len(in.Password) < 8 || len(in.Password) > 72 {
		fail(c, 400, "password must be 8-72 characters")
		return
	}
	if in.Name == "" {
		fail(c, 400, "name is required")
		return
	}
	if strings.TrimSpace(in.OrgName) == "" {
		in.OrgName = in.Name + "'s workspace"
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		internalErr(c, err)
		return
	}
	var u userDTO
	var orgID uuid.UUID
	err = pgx.BeginFunc(c, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO users (email_enc,email_hash,name_enc,password_hash) VALUES ($1,$2,$3,$4) RETURNING id`,
			s.pii.Seal("users.email", in.Email), s.pii.Index(in.Email), s.pii.Seal("users.name", in.Name), hash).Scan(&u.ID); err != nil {
			return err
		}
		u.Email, u.Name = in.Email, in.Name
		if err := tx.QueryRow(c, `INSERT INTO orgs (name) VALUES ($1) RETURNING id`, strings.TrimSpace(in.OrgName)).Scan(&orgID); err != nil {
			return err
		}
		_, err := tx.Exec(c, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,'admin')`, orgID, u.ID)
		return err
	})
	if isUnique(err) {
		fail(c, 409, "an account with this email already exists")
		return
	} else if err != nil {
		internalErr(c, err)
		return
	}
	s.record2(c, orgID, u, "auth.register")
	s.issue(c, u)
}

func (s *Server) login(c *gin.Context) {
	var in struct{ Email, Password string }
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, 400, "invalid request")
		return
	}
	var u userDTO
	var hash, emailEnc, nameEnc string
	err := s.db.QueryRow(c, `SELECT id,email_enc,name_enc,password_hash FROM users WHERE email_hash=$1`,
		s.pii.Index(in.Email)).Scan(&u.ID, &emailEnc, &nameEnc, &hash)
	if err == nil {
		u, err = s.openUser(u.ID, emailEnc, nameEnc)
	}
	if err != nil {
		auth.CheckPassword(s.dummyHash, in.Password) // equalize timing
		fail(c, 401, "invalid email or password")
		return
	}
	if !auth.CheckPassword(hash, in.Password) {
		fail(c, 401, "invalid email or password")
		return
	}
	var orgID uuid.UUID
	if s.db.QueryRow(c, `SELECT org_id FROM memberships WHERE user_id=$1 ORDER BY created_at LIMIT 1`, u.ID).Scan(&orgID) == nil {
		s.record2(c, orgID, u, "auth.login")
	}
	s.issue(c, u)
}

func (s *Server) record2(c *gin.Context, orgID uuid.UUID, u userDTO, action string) {
	s.audit.Publish(c, auditEvent(orgID, u, action, c.ClientIP()))
}

func (s *Server) refresh(c *gin.Context) {
	tok, err := c.Cookie(refreshCookie)
	if err != nil || tok == "" {
		fail(c, 401, "no session")
		return
	}
	uid, err := s.auth.ConsumeRefreshToken(c, tok)
	if err != nil {
		s.setRefreshCookie(c, "", -1)
		fail(c, 401, "session expired")
		return
	}
	u, err := s.loadUser(c, uid)
	if err != nil {
		fail(c, 401, "session expired")
		return
	}
	s.issue(c, u) // rotates the refresh token
}

func (s *Server) logout(c *gin.Context) {
	if tok, err := c.Cookie(refreshCookie); err == nil {
		s.auth.RevokeRefreshToken(c, tok)
	}
	s.setRefreshCookie(c, "", -1)
	c.Status(204)
}

func (s *Server) me(c *gin.Context) {
	u, err := s.loadUser(c, me(c).UserID)
	if err != nil {
		fail(c, 401, "unauthorized")
		return
	}
	c.JSON(200, u)
}
