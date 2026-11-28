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
	ErrCompareEnv      = apperr.Forbidden(MsgCompareEnv)
	ErrCompareTooFew   = apperr.Invalid(MsgCompareTooFew)

	ErrProtectedEnv        = apperr.Forbidden(MsgProtectedEnv)
	ErrChangeOpen          = apperr.Conflict(MsgChangeOpen)
	ErrChangeNotFound      = apperr.NotFound(MsgChangeNotFound)
	ErrChangeNotPending    = apperr.Conflict(MsgChangeNotPending)
	ErrStaleChange         = apperr.Conflict(MsgStaleChange)
	ErrSelfApproval        = apperr.Forbidden(MsgSelfApproval)
	ErrSelfReview          = apperr.Forbidden(MsgSelfReview)
	ErrNotAuthor           = apperr.Forbidden(MsgNotAuthor)
	ErrInvalidChangeStatus = apperr.Invalid(MsgInvalidChangeStatus)
)
