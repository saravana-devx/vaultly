package secret

const (
	MsgSecretsFetched  = "Secrets fetched"
	MsgSecretFetched   = "Secret fetched"
	MsgSecretSaved     = "Secret saved"
	MsgSecretDeleted   = "Secret deleted"
	MsgVersionsFetched = "Versions fetched"
	MsgRolledBack      = "Secret restored to the selected version"
	MsgRotated         = "Secret rotated"
	MsgImported        = "Import finished"
	MsgExported        = "Secrets exported"
	MsgSearchDone      = "Search finished"

	MsgSecretNotFound  = "Secret not found"
	MsgVersionNotFound = "Version not found"
	MsgInvalidKeyPath  = "Invalid key or path (keys: letters, digits, _ . -)"
	MsgInvalidPath     = "Invalid path"
	MsgInvalidEnvPath  = "Invalid environment or path"
	MsgValueTooLarge   = "Value is too large"
	MsgExpiryPast      = "Expiry must be in the future"
	MsgInvalidTags     = "Tags: up to 10, lowercase letters, digits, - and _"
	MsgNoPairs         = "No KEY=VALUE pairs found"
	MsgReadOnlyToken   = "This token is read-only"
	MsgScopeFmt        = "Path is outside this token's scope (%s)"
)
