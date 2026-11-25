package dynamic

import "vaultly/internal/apperr"

var (
	ErrNotFound      = apperr.NotFound(MsgNotFound)
	ErrNameTaken     = apperr.Conflict(MsgNameTaken)
	ErrInvalidName   = apperr.Invalid(MsgInvalidName)
	ErrInvalidTTL    = apperr.Invalid(MsgInvalidTTL)
	ErrEnvNotFound   = apperr.Invalid(MsgEnvNotFound)
	ErrTokenNotFound = apperr.NotFound(MsgTokenNotFound)
)
