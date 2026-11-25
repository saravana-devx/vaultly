package invite

import "vaultly/internal/apperr"

var (
	ErrInvalidEmail   = apperr.Invalid(MsgInvalidEmail)
	ErrInvalidEnvRole = apperr.Invalid(MsgInvalidEnvRole)
	ErrAlreadyMember  = apperr.Conflict(MsgAlreadyMember)
	ErrInviteNotFound = apperr.NotFound(MsgInviteNotFound)
	ErrInviteUsed     = apperr.Gone(MsgInviteUsed)
)
