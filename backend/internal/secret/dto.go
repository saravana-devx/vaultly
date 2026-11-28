package secret

import "time"

type PutRequest struct {
	Env         string     `json:"env" binding:"required"`
	Path        string     `json:"path"`
	Key         string     `json:"key" binding:"required"`
	Value       string     `json:"value"`
	Comment     string     `json:"comment"`
	Tags        *[]string  `json:"tags"`
	ExpiresAt   *time.Time `json:"expires_at"`
	ClearExpiry bool       `json:"clear_expiry"`
}

type ImportRequest struct {
	Env     string `json:"env" binding:"required"`
	Path    string `json:"path"`
	Content string `json:"content" binding:"required"`
}

type RollbackRequest struct {
	Version int `json:"version" binding:"required,min=1"`
}

type RotateRequest struct {
	Length int `json:"length"`
}

type MachinePutRequest struct {
	Path    string `json:"path"`
	Key     string `json:"key" binding:"required"`
	Value   string `json:"value"`
	Comment string `json:"comment"`
}

// Query describes a folder listing.
type Query struct {
	Env  string
	Path string
	Tag  string
}

// ExportQuery describes an export of one environment path.
type ExportQuery struct {
	Env       string
	Path      string
	Recursive bool
	Resolve   bool
}

type DecideRequest struct {
	Note string `json:"note" binding:"max=500"`
}
