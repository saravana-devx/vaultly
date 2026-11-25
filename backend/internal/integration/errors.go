package integration

import "vaultly/internal/apperr"

var (
	ErrNotFound        = apperr.NotFound(MsgNotFound)
	ErrUnknownProvider = apperr.Invalid(MsgUnknownProvider)
	ErrInvalidEnvPath  = apperr.Invalid(MsgInvalidEnvPath)
)
