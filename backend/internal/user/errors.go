package user

import "vaultly/internal/apperr"

var (
	ErrEmailTaken         = apperr.Conflict(MsgEmailTaken)
	ErrInvalidEmail       = apperr.Invalid(MsgInvalidEmail)
	ErrNameRequired       = apperr.Invalid(MsgNameRequired)
	ErrInvalidCredentials = apperr.Unauthorized(MsgInvalidCredentials)
	ErrNoSession          = apperr.Unauthorized(MsgNoSession)
	ErrSessionExpired     = apperr.Unauthorized(MsgSessionExpired)
	ErrUserNotFound       = apperr.NotFound(MsgUserNotFound)
)
