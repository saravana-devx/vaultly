package project

type CreateProjectRequest struct {
	Name string `json:"name" binding:"required,max=80"`
}

type CreateEnvRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug"`
}

type UpdateEnvRequest struct {
	Protected *bool `json:"protected" binding:"required"`
}
