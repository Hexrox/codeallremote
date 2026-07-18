// Package errcat defines the stable CAR API error catalog.
//
// All API errors flow through this catalog so that clients (notably Android)
// can branch on a stable code rather than parsing prose. Each code maps to a
// fixed HTTP status and a generic, secret-free message.
package errcat

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is a stable, machine-readable error code.
type Code string

// Stable error codes. Additions are additive; existing codes never change
// meaning or HTTP status. New codes require a fixture and a release note.
const (
	// Unauthorized: missing or invalid authentication.
	Unauthorized Code = "unauthorized"
	// Forbidden: authenticated but lacking permission for this action.
	Forbidden Code = "forbidden"
	// NotFound: resource does not exist or is not visible to this actor.
	NotFound Code = "not_found"
	// Conflict: state or idempotency conflict.
	Conflict Code = "conflict"
	// ExpiredApproval: an approval decision targeted a no-longer-pending approval.
	ExpiredApproval Code = "expired_approval"
	// IdempotencyConflict: an idempotency key matched a different request body.
	IdempotencyConflict Code = "idempotency_conflict"
	// CursorExpired: a replay cursor pointed past retained events.
	CursorExpired Code = "cursor_expired"
	// AdapterUnavailable: the requested adapter is not registered or not healthy.
	AdapterUnavailable Code = "adapter_unavailable"
	// InvalidInput: request body or parameters failed validation.
	InvalidInput Code = "invalid_input"
	// rateLimited: client sent commands too fast (reserved for future).
	RateLimited Code = "rate_limited"
	// Internal: unexpected server error; no details leak.
	Internal Code = "internal_error"
	// UnsupportedCommand: command type is not known to this server.
	UnsupportedCommand Code = "unsupported_command"
)

// httpStatus maps each code to its HTTP status. Centralized so the mapping
// is reviewed in one place when codes are added.
var httpStatus = map[Code]int{
	Unauthorized:        http.StatusUnauthorized,
	Forbidden:           http.StatusForbidden,
	NotFound:            http.StatusNotFound,
	Conflict:            http.StatusConflict,
	ExpiredApproval:     http.StatusConflict,
	IdempotencyConflict: http.StatusConflict,
	CursorExpired:       http.StatusConflict,
	AdapterUnavailable:  http.StatusServiceUnavailable,
	InvalidInput:        http.StatusBadRequest,
	RateLimited:         http.StatusTooManyRequests,
	Internal:            http.StatusInternalServerError,
	UnsupportedCommand:  http.StatusBadRequest,
}

// HTTPStatus returns the HTTP status for a code.
func HTTPStatus(c Code) int {
	if s, ok := httpStatus[c]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// Error is a typed API error carrying a stable code and a secret-free message.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
	cause   error  `json:"-"`
}

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is allows errors.Is(err, errcat.NotFound) to match by code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// New returns a new typed error with a generic message for the code.
// Detailed, secret-free context may be attached via WithDetails.
func New(c Code, message string) *Error {
	if message == "" {
		message = defaultMessage(c)
	}
	return &Error{Code: c, Message: message}
}

// WithDetails attaches structured, secret-free details.
func (e *Error) WithDetails(d any) *Error {
	e.Details = d
	return e
}

// defaultMessage returns the canonical, client-stable message for a code.
// The message MUST NOT include secrets, paths, or raw request content.
func defaultMessage(c Code) string {
	switch c {
	case Unauthorized:
		return "Authentication required."
	case Forbidden:
		return "This action is not permitted."
	case NotFound:
		return "Resource not found."
	case Conflict:
		return "State conflict."
	case ExpiredApproval:
		return "Approval is no longer pending."
	case IdempotencyConflict:
		return "Idempotency key was used for a different request."
	case CursorExpired:
		return "Replay cursor is past retained events."
	case AdapterUnavailable:
		return "The requested adapter is unavailable."
	case InvalidInput:
		return "Request was invalid."
	case RateLimited:
		return "Too many requests."
	case UnsupportedCommand:
		return "Command is not supported."
	default:
		return "Internal error."
	}
}

// Wrap returns an *Error with the given code and a cause attached for logging
// only (the cause is never serialized to the client).
func Wrap(c Code, cause error, message string) *Error {
	e := New(c, message)
	e.cause = cause
	return e
}

// Cause returns the underlying wrapped error (for server logs only).
func (e *Error) Cause() error { return e.cause }

// CodeFrom returns the Code carried by err, or "" if err is not a catalog error.
func CodeFrom(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// FromAppError maps the app-layer sentinel errors to catalog codes.
// Unknown errors become Internal.
func FromAppError(err error, appNotFound, appConflict, appInvalid error) *Error {
	switch {
	case errors.Is(err, appNotFound):
		return New(NotFound, err.Error())
	case errors.Is(err, appConflict):
		return New(Conflict, err.Error())
	case errors.Is(err, appInvalid):
		return New(InvalidInput, err.Error())
	default:
		return Wrap(Internal, err, "")
	}
}
