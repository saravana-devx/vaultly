package dynamic

type CreateRequest struct {
	Name       string   `json:"name" binding:"required,max=60"`
	Env        string   `json:"env" binding:"required"`
	AdminURL   string   `json:"admin_url" binding:"required"`
	Grants     []string `json:"grants"`
	DefaultTTL int      `json:"default_ttl_s"`
	MaxTTL     int      `json:"max_ttl_s"`
}

type LeaseRequest struct {
	TTL int `json:"ttl_seconds"`
}
