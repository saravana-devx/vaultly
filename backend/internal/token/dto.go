package token

type CreateServiceRequest struct {
	Name         string   `json:"name" binding:"required,max=80"`
	Env          string   `json:"env" binding:"required"`
	Path         string   `json:"path"`
	CanWrite     bool     `json:"can_write"`
	ExpireDays   int      `json:"expire_days" binding:"min=0,max=3650"`
	AllowedCIDRs []string `json:"allowed_cidrs"`
}

type CreatePersonalRequest struct {
	Name       string `json:"name" binding:"required,max=80"`
	ExpireDays int    `json:"expire_days" binding:"min=0,max=3650"`
}
