package backup

import (
	"time"

	"github.com/google/uuid"
)

type exportVersion struct {
	Version        int       `json:"version"`
	EncryptedValue []byte    `json:"encrypted_value"`
	Action         string    `json:"action"`
	Actor          string    `json:"actor"`
	CreatedAt      time.Time `json:"created_at"`
}

type exportSecret struct {
	Env            string          `json:"env"`
	Path           string          `json:"path"`
	Key            string          `json:"key"`
	EncryptedValue []byte          `json:"encrypted_value"`
	Version        int             `json:"version"`
	Comment        string          `json:"comment"`
	Tags           []string        `json:"tags"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	UpdatedBy      string          `json:"updated_by"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Versions       []exportVersion `json:"versions"`

	id uuid.UUID
}

type exportEnv struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
}

type exportProject struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	EncryptedDEK []byte         `json:"encrypted_dek"`
	CreatedAt    time.Time      `json:"created_at"`
	Environments []exportEnv    `json:"environments"`
	Secrets      []exportSecret `json:"secrets"`
}

const formatVersion = 1

type exportDoc struct {
	Format     int             `json:"format"`
	ExportedAt time.Time       `json:"exported_at"`
	OrgName    string          `json:"org_name"`
	Projects   []exportProject `json:"projects"`
}

type Skipped struct {
	Project string `json:"project"`
	Reason  string `json:"reason"`
}

type RestoreResult struct {
	Restored int       `json:"restored"`
	Secrets  int       `json:"secrets"`
	Skipped  []Skipped `json:"skipped"`
}

// File is a finished backup ready to download.
type File struct {
	Name string
	Data []byte
}
