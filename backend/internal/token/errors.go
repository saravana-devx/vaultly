package token

import "vaultly/internal/apperr"

var (
	ErrNameRequired   = apperr.Invalid(MsgNameRequired)
	ErrInvalidEnvPath = apperr.Invalid(MsgInvalidEnvPath)
	ErrInvalidCIDRs   = apperr.Invalid(MsgInvalidCIDRs)
	ErrTokenNotFound  = apperr.NotFound(MsgTokenNotFound)
	ErrNeedPassword   = apperr.Forbidden(MsgNeedPassword)
	ErrInvalidToken   = apperr.Unauthorized(MsgInvalidToken)
	ErrNeedService    = apperr.Unauthorized(MsgNeedService)
	ErrIPNotAllowed   = apperr.Forbidden(MsgIPNotAllowed)
)
