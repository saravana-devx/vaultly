package token

const (
	MsgServiceListed   = "Service tokens fetched"
	MsgServiceCreated  = "Service token created. Copy it now, it will not be shown again"
	MsgServiceRevoked  = "Service token revoked"
	MsgPersonalListed  = "Personal tokens fetched"
	MsgPersonalCreated = "Personal token created. Copy it now, it will not be shown again"
	MsgPersonalRevoked = "Personal token revoked"

	MsgNameRequired   = "Name is required"
	MsgInvalidEnvPath = "Invalid environment or path"
	MsgInvalidCIDRs   = "Allowed IPs must be valid addresses or CIDR ranges (max 20)"
	MsgTokenNotFound  = "Token not found"
	MsgNeedPassword   = "Sign in with your password to manage personal tokens"
	MsgInvalidToken   = "Invalid or expired token"
	MsgNeedService    = "A service token is required"
	MsgIPNotAllowed   = "This token cannot be used from your IP address"
)
