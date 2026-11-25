package dynamic

import (
	"time"

	"github.com/google/uuid"
)

type View struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	Provider     string         `json:"provider"`
	Env          string         `json:"env"`
	Target       map[string]any `json:"target"`
	Grants       []string       `json:"grants"`
	DefaultTTL   int            `json:"default_ttl_s"`
	MaxTTL       int            `json:"max_ttl_s"`
	ActiveLeases int            `json:"active_leases"`
}

// ConfigRow is a dynamic secret definition as stored (config still encrypted).
type ConfigRow struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Name       string
	Provider   string
	Env        string
	ConfigEnc  []byte
	DefaultTTL int
	MaxTTL     int
	Active     int
}

type NewConfig struct {
	ProjectID, EnvID uuid.UUID
	Name             string
	ConfigEnc        []byte
	DefaultTTL       int
	MaxTTL           int
}

type Created struct {
	ID uuid.UUID `json:"id"`
}

// Lease is the credential handed to the caller. The password is shown once.
type Lease struct {
	ID        uuid.UUID `json:"lease_id"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URL       string    `json:"connection_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type LeaseView struct {
	ID          uuid.UUID  `json:"id"`
	Username    string     `json:"username"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	State       string     `json:"state"`
	RevokeError string     `json:"revoke_error"`
}

// LeaseTarget is what revoking a lease needs.
type LeaseTarget struct {
	Username  string
	ConfigEnc []byte
	ProjectID uuid.UUID
}
