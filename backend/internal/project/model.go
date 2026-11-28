package project

import (
	"time"

	"github.com/google/uuid"
)

type Env struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Protected bool      `json:"protected"` // changes by non-admins need approval
}

type Summary struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	CreatedAt    time.Time  `json:"created_at"`
	SecretCount  int        `json:"secret_count"`
	LastActivity *time.Time `json:"last_activity"`
	Environments []Env      `json:"environments"`
}

type Detail struct {
	ID           uuid.UUID `json:"id"`
	OrgID        uuid.UUID `json:"org_id"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Environments []Env     `json:"environments"`
}

type Created struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Environments []Env     `json:"environments"`
}

var defaultEnvs = []Env{{Name: "Development", Slug: "dev"}, {Name: "Staging", Slug: "staging"}, {Name: "Production", Slug: "prod"}}
