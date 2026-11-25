package member

import (
	"time"

	"github.com/google/uuid"
)

type Member struct {
	ID       uuid.UUID         `json:"id"`
	Email    string            `json:"email"`
	Name     string            `json:"name"`
	Role     string            `json:"role"`
	JoinedAt time.Time         `json:"joined_at"`
	EnvRoles map[string]string `json:"env_roles"`
}

// Row is a member as stored (email and name still sealed).
type Row struct {
	ID       uuid.UUID
	EmailEnc string
	NameEnc  string
	Role     string
	JoinedAt time.Time
}
