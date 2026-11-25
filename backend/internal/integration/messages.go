package integration

const (
	MsgListed   = "Integrations fetched"
	MsgCreated  = "Integration connected"
	MsgUpdated  = "Integration updated"
	MsgDeleted  = "Integration removed"
	MsgSynced   = "Secrets pushed to the provider"
	MsgNotFound = "Integration not found"

	MsgUnknownProvider = "Provider must be github, vercel or railway"
	MsgInvalidEnvPath  = "Invalid environment or path"
)
