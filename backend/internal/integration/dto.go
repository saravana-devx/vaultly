package integration

type CreateRequest struct {
	Provider string            `json:"provider" binding:"required"`
	Name     string            `json:"name" binding:"required,max=80"`
	Env      string            `json:"env" binding:"required"`
	Path     string            `json:"path"`
	Config   map[string]string `json:"config" binding:"required"`
	AutoSync bool              `json:"auto_sync"`
}

type UpdateRequest struct {
	AutoSync *bool `json:"auto_sync" binding:"required"`
}
