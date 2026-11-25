package member

import "vaultly/internal/apperr"

var (
	ErrAlreadyMember  = apperr.Conflict(MsgAlreadyMember)
	ErrLastAdmin      = apperr.Conflict(MsgLastAdmin)
	ErrMemberNotFound = apperr.NotFound(MsgMemberNotFound)
	ErrInvalidEnvRole = apperr.Invalid(MsgInvalidEnvRole)
)
