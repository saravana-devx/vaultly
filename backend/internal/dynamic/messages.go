package dynamic

const (
	MsgListed        = "Dynamic secrets fetched"
	MsgCreated       = "Dynamic secret created"
	MsgDeleted       = "Dynamic secret deleted"
	MsgLeased        = "Credentials issued. Copy the password now, it will not be shown again"
	MsgLeasesListed  = "Leases fetched"
	MsgRevoked       = "Credentials revoked"
	MsgNotFound      = "Dynamic secret not found"
	MsgNameTaken     = "A dynamic secret with that name already exists in this environment"
	MsgInvalidName   = "Name is required"
	MsgInvalidTTL    = "TTLs must be at least 60s, default ≤ max, max ≤ 30 days"
	MsgEnvNotFound   = "Environment not found"
	MsgTokenNotFound = "Dynamic secret not found in this token's environment"
)
