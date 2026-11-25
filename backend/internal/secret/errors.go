package secret

import "vaultly/internal/apperr"

var (
	ErrSecretNotFound  = apperr.NotFound(MsgSecretNotFound)
	ErrVersionNotFound = apperr.NotFound(MsgVersionNotFound)
	ErrInvalidKeyPath  = apperr.Invalid(MsgInvalidKeyPath)
	ErrInvalidPath     = apperr.Invalid(MsgInvalidPath)
	ErrInvalidEnvPath  = apperr.Invalid(MsgInvalidEnvPath)
	ErrValueTooLarge   = apperr.Invalid(MsgValueTooLarge)
	ErrExpiryPast      = apperr.Invalid(MsgExpiryPast)
	ErrInvalidTags     = apperr.Invalid(MsgInvalidTags)
	ErrNoPairs         = apperr.Invalid(MsgNoPairs)
	ErrReadOnlyToken   = apperr.Forbidden(MsgReadOnlyToken)
)
