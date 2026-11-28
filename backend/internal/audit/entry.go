package audit

import (
	"time"

	"github.com/google/uuid"
)

// Entry is one stored event as shown in the activity log.
type Entry struct {
	ID        uuid.UUID      `json:"id"`
	ProjectID *uuid.UUID     `json:"project_id"`
	Project   *string        `json:"project"`
	ActorType string         `json:"actor_type"`
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	Resource  string         `json:"resource"`
	Metadata  map[string]any `json:"metadata"`
	IP        string         `json:"ip"`
	At        time.Time      `json:"at"`
}

type Page struct {
	Events []Entry `json:"events"`
	Next   string  `json:"next"`
}

type ListQuery struct {
	Before    time.Time
	BeforeID  *uuid.UUID // tie-breaker for events that share Before's timestamp
	ProjectID *uuid.UUID
	Kind      string // action prefix such as "secret" matches secret.create, secret.read, ...
	Limit     int
}

const MsgFetched = "Activity fetched"
