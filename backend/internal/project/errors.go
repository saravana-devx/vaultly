package project

import "vaultly/internal/apperr"

var (
	ErrEnvExists      = apperr.Conflict(MsgEnvExists)
	ErrEnvNotFound    = apperr.NotFound(MsgEnvNotFound)
	ErrInvalidEnvSlug = apperr.Invalid(MsgInvalidEnvSlug)
	ErrNameRequired   = apperr.Invalid(MsgNameRequired)
)
