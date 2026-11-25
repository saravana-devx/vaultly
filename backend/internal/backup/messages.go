package backup

const (
	MsgRestored = "Backup restored"

	MsgPassphrase  = "Choose a passphrase of at least 12 characters"
	MsgNoFile      = "Attach the backup file"
	MsgUnreadable  = "Could not read the file"
	MsgBadFormat   = "Unsupported backup format"
	MsgOrgNotFound = "Workspace not found"

	ReasonExists   = "a project with this ID already exists"
	ReasonWrongKey = "was encrypted with a different MASTER_KEY"
	ReasonRollback = "restore failed and was rolled back"
)
