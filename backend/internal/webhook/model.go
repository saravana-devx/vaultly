package webhook

import (
	"time"

	"github.com/google/uuid"
)

// Hook is a webhook ready to deliver to (secret already decrypted).
type Hook struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	URL       string
	Secret    string
	Events    []string
}

// HookRow is a webhook as stored (secret still encrypted with the project key).
type HookRow struct {
	ID        uuid.UUID
	Name      string
	URL       string
	SecretEnc []byte
	Events    []string
}

type Result struct {
	Status   int
	Error    string
	Attempts int
	Duration time.Duration
}

type View struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Events         []string   `json:"events"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
	LastStatus     *int       `json:"last_status"`
	LastError      *string    `json:"last_error"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
}

type Created struct {
	ID     uuid.UUID `json:"id"`
	Secret string    `json:"secret"`
}

type TestResult struct {
	Status     int    `json:"status"`
	Error      string `json:"error"`
	DurationMS int64  `json:"duration_ms"`
}

type Delivery struct {
	Action     string    `json:"action"`
	Status     int       `json:"status"`
	Error      string    `json:"error"`
	Attempts   int       `json:"attempts"`
	DurationMS int       `json:"duration_ms"`
	At         time.Time `json:"at"`
}

// Update carries only the fields the caller wants changed.
type Update struct {
	Name   *string
	URL    *string
	Events *[]string
	Active *bool
}
