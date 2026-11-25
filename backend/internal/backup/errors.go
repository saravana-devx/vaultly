package backup

import "vaultly/internal/apperr"

var (
	ErrPassphrase  = apperr.Invalid(MsgPassphrase)
	ErrNoFile      = apperr.Invalid(MsgNoFile)
	ErrUnreadable  = apperr.Invalid(MsgUnreadable)
	ErrBadFormat   = apperr.Invalid(MsgBadFormat)
	ErrOrgNotFound = apperr.NotFound(MsgOrgNotFound)
)
