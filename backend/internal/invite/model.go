package invite

import (
	"time"

	"github.com/google/uuid"
)

// Invite is a pending invitation as stored; email and inviter are still sealed.
type Invite struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	OrgName   string
	EmailEnc  string
	Role      string
	EnvRoles  map[string]string
	InvitedBy string
	ExpiresAt time.Time
	CreatedAt time.Time
	Accepted  *time.Time
}

type NewInvite struct {
	OrgID     uuid.UUID
	EmailEnc  string
	EmailHash []byte
	Role      string
	EnvRoles  map[string]string
	TokenHash []byte
	InvitedBy string
	ExpiresAt time.Time
}

// View is the decrypted listing entry shown to admins.
type View struct {
	ID        uuid.UUID         `json:"id"`
	Email     string            `json:"email"`
	Role      string            `json:"role"`
	EnvRoles  map[string]string `json:"env_roles"`
	InvitedBy string            `json:"invited_by"`
	ExpiresAt time.Time         `json:"expires_at"`
	CreatedAt time.Time         `json:"created_at"`
	Expired   bool              `json:"expired"`
}

type Created struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
	Link  string    `json:"link"`
}

// Preview is what anyone holding the link may see before signing in.
type Preview struct {
	Org        string `json:"org"`
	EmailHint  string `json:"email_hint"`
	Role       string `json:"role"`
	InvitedBy  string `json:"invited_by"`
	State      string `json:"state"`
	HasAccount bool   `json:"has_account"`
}

type Accepted struct {
	OrgID uuid.UUID `json:"org_id"`
	Org   string    `json:"org"`
	Role  string    `json:"role"`
}
