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

	ErrInvalidMFACode      = apperr.Unauthorized(MsgInvalidMFACode)
	ErrMFAChallengeExpired = apperr.Unauthorized(MsgMFAChallengeExpired)
	ErrMFANotEnabled       = apperr.Invalid(MsgMFANotEnabled)
	ErrMFAAlreadyOn        = apperr.Conflict(MsgMFAAlreadyOn)
	ErrMFANoSetup          = apperr.Invalid(MsgMFANoSetup)
	ErrMFANeedSession      = apperr.Forbidden(MsgMFANeedSession)
	ErrWrongPassword       = apperr.Unauthorized(MsgWrongPassword)
)
