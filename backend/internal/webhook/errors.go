package webhook

import "vaultly/internal/apperr"

var (
	ErrNotFound      = apperr.NotFound(MsgNotFound)
	ErrInvalidURL    = apperr.Invalid(MsgInvalidURL)
	ErrPrivateURL    = apperr.Invalid(MsgPrivateURL)
	ErrInvalidEvents = apperr.Invalid(MsgInvalidEvents)
)
