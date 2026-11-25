package api

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Users keep email and name encrypted (internal/pii); lookups use the blind index.

func (s *Server) sealLabel(l string) string { return s.pii.Label(l) }
func (s *Server) openLabel(l string) string { return s.pii.OpenLabel(l) }

func (s *Server) openUser(id uuid.UUID, emailEnc, nameEnc string) (userDTO, error) {
	email, err := s.pii.Open("users.email", emailEnc)
	if err != nil {
		return userDTO{}, err
	}
	name, err := s.pii.Open("users.name", nameEnc)
	if err != nil {
		return userDTO{}, err
	}
	return userDTO{ID: id, Email: email, Name: name}, nil
}

func (s *Server) loadUser(ctx context.Context, id uuid.UUID) (userDTO, error) {
	var e, n string
	if err := s.db.QueryRow(ctx, `SELECT email_enc, name_enc FROM users WHERE id=$1`, id).Scan(&e, &n); err != nil {
		return userDTO{}, err
	}
	return s.openUser(id, e, n)
}

func (s *Server) userIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT id FROM users WHERE email_hash=$1`, s.pii.Index(email)).Scan(&id)
	return id, err
}

func (s *Server) emailExists(ctx context.Context, email string) bool {
	_, err := s.userIDByEmail(ctx, email)
	return err == nil
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
