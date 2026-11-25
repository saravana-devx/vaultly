package invite

type CreateInviteRequest struct {
	Email    string            `json:"email" binding:"required"`
	Role     string            `json:"role" binding:"required,oneof=admin developer viewer"`
	EnvRoles map[string]string `json:"env_roles"`
}
