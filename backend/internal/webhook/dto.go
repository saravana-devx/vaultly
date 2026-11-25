package webhook

type CreateRequest struct {
	Name   string   `json:"name" binding:"required,max=80"`
	URL    string   `json:"url" binding:"required"`
	Events []string `json:"events"`
}

type UpdateRequest struct {
	Name   *string   `json:"name"`
	URL    *string   `json:"url"`
	Events *[]string `json:"events"`
	Active *bool     `json:"active"`
}
