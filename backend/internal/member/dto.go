package member

type AddMemberRequest struct {
	Email string `json:"email" binding:"required"`
	Role  string `json:"role" binding:"required,oneof=admin developer viewer"`
}

type UpdateRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin developer viewer"`
}

// EnvRolesRequest maps environment name -> none | viewer | developer | inherit.
type EnvRolesRequest struct {
	Roles map[string]string `json:"roles" binding:"required"`
}

type EnvRolesResponse struct {
	Roles map[string]string `json:"roles"`
}
