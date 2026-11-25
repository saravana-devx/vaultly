package integration

import (
	"time"

	"github.com/google/uuid"
)

type View struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	Env          string            `json:"env"`
	Path         string            `json:"path"`
	AutoSync     bool              `json:"auto_sync"`
	Config       map[string]string `json:"config"`
	LastSyncedAt *time.Time        `json:"last_synced_at"`
	LastStatus   string            `json:"last_status"`
	LastError    string            `json:"last_error"`
}

// Row is an integration as stored (config still encrypted).
type Row struct {
	View
	ConfigEnc []byte
}

// SyncInfo is everything a sync needs about one integration.
type SyncInfo struct {
	ID, ProjectID, OrgID, EnvID uuid.UUID
	Name, Provider, EnvSlug     string
	Path                        string
	ConfigEnc                   []byte
}

type NewIntegration struct {
	ProjectID, EnvID uuid.UUID
	Path, Provider   string
	Name             string
	ConfigEnc        []byte
	AutoSync         bool
}

type Created struct {
	ID uuid.UUID `json:"id"`
}

type SyncResult struct {
	Pushed  int      `json:"pushed"`
	Skipped []string `json:"skipped"`
}
