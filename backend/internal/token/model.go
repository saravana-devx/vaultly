package token

import (
	"time"

	"github.com/google/uuid"
)

// ServiceView is a project service token as listed to admins (never includes the secret).
type ServiceView struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Prefix       string     `json:"prefix"`
	Env          string     `json:"env"`
	Path         string     `json:"path"`
	CanWrite     bool       `json:"can_write"`
	AllowedCIDRs []string   `json:"allowed_cidrs"`
	ExpiresAt    *time.Time `json:"expires_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

type PersonalView struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Issued is returned once, at creation: the raw token is never retrievable again.
type Issued struct {
	ID    uuid.UUID `json:"id"`
	Token string    `json:"token"`
	Name  string    `json:"name"`
}

type NewServiceToken struct {
	ProjectID, EnvID uuid.UUID
	Name, Prefix     string
	Hash             []byte
	Path             string
	CanWrite         bool
	CIDRs            []string
	ExpiresAt        *time.Time
	CreatedBy        string
}

type NewPersonalToken struct {
	UserID    uuid.UUID
	Name      string
	Prefix    string
	Hash      []byte
	ExpiresAt *time.Time
}

// ServiceRecord is a service token joined with the context needed to authorise a request.
type ServiceRecord struct {
	ID, ProjectID, OrgID, EnvID uuid.UUID
	EnvSlug, Name, PathPrefix   string
	CanWrite                    bool
	CIDRs                       []string
	ExpiresAt                   *time.Time
}

type PersonalRecord struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	EmailEnc  string
	ExpiresAt *time.Time
}
