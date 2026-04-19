// Package apierror defines the typed errors that services return and
// middleware translates to HTTP responses.
//
// A service that wants to signal "not found" returns apierror.ErrNotFound.
// The handler catches the error with c.Error(err), and the ErrorHandler
// middleware (see internal/shared/middleware/errorhandler.go) unwraps it
// via apierror.As and sends the matching HTTP status through
// response.Error.
//
// When you need a new error type (e.g. ErrForbidden, ErrTooManyRequests),
// add it here with a clear Code, user-safe Message, and correct HTTPCode.
// Never put error codes anywhere else in the codebase — this is the single
// source of truth.
package apierror

import "errors"

type Error struct {
	Code     string
	Message  string
	HTTPCode int
}

func (e *Error) Error() string { return e.Message }

var (
	ErrNotFound   = &Error{Code: "NOT_FOUND", Message: "resource not found", HTTPCode: 404}
	ErrConflict   = &Error{Code: "CONFLICT", Message: "resource already exists", HTTPCode: 409}
	ErrBadRequest = &Error{Code: "BAD_REQUEST", Message: "bad request", HTTPCode: 400}
	ErrInternal   = &Error{Code: "INTERNAL", Message: "internal server error", HTTPCode: 500}
)

// As unwraps an error into an *apierror.Error if possible. Middleware uses
// this to decide the HTTP status for an error returned from a handler.
func As(err error) (*Error, bool) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
