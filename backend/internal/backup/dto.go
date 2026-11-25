package backup

type CreateRequest struct {
	Passphrase string `json:"passphrase" binding:"required,min=12"`
}
