package secret

import (
	"time"

	"github.com/google/uuid"
)

type Secret struct {
	ID        uuid.UUID  `json:"id"`
	Key       string     `json:"key"`
	Path      string     `json:"path"`
	Version   int        `json:"version"`
	Comment   string     `json:"comment"`
	Tags      []string   `json:"tags"`
	ExpiresAt *time.Time `json:"expires_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	UpdatedBy string     `json:"updated_by"`
	Value     *string    `json:"value,omitempty"`
	Resolved  *string    `json:"resolved,omitempty"`
	Env       string     `json:"env,omitempty"`
}

// Stored is a secret together with its ciphertext, as read for reveal/rollback.
type Stored struct {
	Secret
	Ciphertext []byte
}

// Write is everything needed to create or update one secret (value already encrypted).
type Write struct {
	ProjectID, EnvID uuid.UUID
	Path, Key        string
	Ciphertext       []byte
	Comment          string
	Actor            string // already sealed
	Action           string // create | update | rollback | rotate; empty = infer
	Tags             *[]string
	Expires          *time.Time
	ClearExpiry      bool
}

type Listing struct {
	Secrets []Secret `json:"secrets"`
	Folders []string `json:"folders"`
	Path    string   `json:"path"`
}

type SearchResults struct {
	Results []Secret `json:"results"`
}

type VersionInfo struct {
	Version int       `json:"version"`
	Action  string    `json:"action"`
	Actor   string    `json:"actor"`
	At      time.Time `json:"at"`
}

type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type MachineWriteResult struct {
	Key     string `json:"key"`
	Path    string `json:"path"`
	Version int    `json:"version"`
}

// Pair is one decrypted KEY, VALUE.
type Pair [2]string

// LiveRow is a non-expired secret as read for export and API fetches.
type LiveRow struct {
	Path       string
	Key        string
	Ciphertext []byte
}

type Expired struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	Path      string
	Key       string
	Env       string
}
