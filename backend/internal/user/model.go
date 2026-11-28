package user

import (
	"time"

	"github.com/google/uuid"
)

// Profile is the decrypted, public view of an account.
type Profile struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

// Record is a row as stored: personal fields are sealed (see internal/pii).
type Record struct {
	ID           uuid.UUID
	EmailEnc     string
	NameEnc      string
	PasswordHash string
}

type NewUser struct {
	EmailEnc     string
	EmailHash    []byte
	NameEnc      string
	PasswordHash string
	OrgName      string
}

type Session struct {
	AccessToken  string
	RefreshToken string
	User         Profile
	MFAChallenge string // set instead of the tokens when a second factor is still needed
}

// MFARecord is a user's 2FA state as stored; SecretEnc and PendingEnc are sealed.
type MFARecord struct {
	SecretEnc    string
	PendingEnc   string
	LastStep     int64
	EnabledAt    *time.Time
	RecoveryLeft int
}

func (m *MFARecord) Enabled() bool { return m.SecretEnc != "" }
