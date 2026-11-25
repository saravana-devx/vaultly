// Package apperr defines the errors that cross layer boundaries. A service returns one of
// these; httpx turns it into an HTTP status, so handlers never decide status codes themselves.
package apperr

import "errors"

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindGone
	KindUpstream
)

type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string { return e.Message }

func New(kind Kind, message string) *Error { return &Error{Kind: kind, Message: message} }

func Invalid(message string) *Error      { return New(KindInvalid, message) }
func Unauthorized(message string) *Error { return New(KindUnauthorized, message) }
func Forbidden(message string) *Error    { return New(KindForbidden, message) }
func NotFound(message string) *Error     { return New(KindNotFound, message) }
func Conflict(message string) *Error     { return New(KindConflict, message) }
func Gone(message string) *Error         { return New(KindGone, message) }
func Upstream(message string) *Error     { return New(KindUpstream, message) }

func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
