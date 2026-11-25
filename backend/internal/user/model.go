package user

import "github.com/google/uuid"

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
}
