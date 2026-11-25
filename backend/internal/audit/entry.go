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
	ProjectID *uuid.UUID
	Limit     int
}

const MsgFetched = "Activity fetched"
