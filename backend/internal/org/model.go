package org

import (
	"time"

	"github.com/google/uuid"
)

type Org struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Role string    `json:"role"`
}

type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

type RecentEvent struct {
	Actor    string         `json:"actor"`
	Action   string         `json:"action"`
	Resource string         `json:"resource"`
	Project  *string        `json:"project"`
	Metadata map[string]any `json:"metadata"`
	At       time.Time      `json:"at"`
}

type Attention struct {
	Kind      string    `json:"kind"`
	Token     string    `json:"token"`
	ProjectID uuid.UUID `json:"project_id"`
	Project   string    `json:"project"`
	Reason    string    `json:"reason"`
}

type Counts struct {
	Projects int `json:"projects"`
	Secrets  int `json:"secrets"`
	Tokens   int `json:"tokens"`
	Members  int `json:"members"`
	Events24 int `json:"events_24h"`
}

type Stats struct {
	Counts
	Activity       []DayCount    `json:"activity"`
	Recent         []RecentEvent `json:"recent,omitempty"`
	PendingInvites *int          `json:"pending_invites,omitempty"`
	Attention      []Attention   `json:"attention,omitempty"`
}

// ExpiryRow is a raw token/secret expiry the service turns into an attention item.
type ExpiryRow struct {
	Kind      string
	Name      string
	ProjectID uuid.UUID
	Project   string
	ExpiresAt *time.Time
	LastUsed  *time.Time
}
