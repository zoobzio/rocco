package rocco

import "errors"

// Sentinel errors for HTTP response codes.
// These are used to signal successful completion with specific HTTP status codes.
// When a handler or middleware returns one of these errors, the pipeline stops
// but the response is considered successful (not a server error).

// Client errors (4xx)
var (
	// ErrBadRequest indicates the request was invalid (400)
	ErrBadRequest = errors.New("bad request")

	// ErrUnauthorized indicates missing or invalid authentication (401)
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the request is not allowed (403)
	ErrForbidden = errors.New("forbidden")

	// ErrNotFound indicates the resource was not found (404)
	ErrNotFound = errors.New("not found")

	// ErrConflict indicates a conflict with existing data (409)
	ErrConflict = errors.New("conflict")

	// ErrUnprocessableEntity indicates the request was well-formed but semantically invalid (422)
	ErrUnprocessableEntity = errors.New("unprocessable entity")

	// ErrTooManyRequests indicates rate limiting (429)
	ErrTooManyRequests = errors.New("too many requests")
)

// Server errors (5xx)
var (
	// ErrInternalServer indicates an unexpected server error (500)
	ErrInternalServer = errors.New("internal server error")

	// ErrNotImplemented indicates the functionality is not implemented (501)
	ErrNotImplemented = errors.New("not implemented")

	// ErrServiceUnavailable indicates the service is temporarily unavailable (503)
	ErrServiceUnavailable = errors.New("service unavailable")
)
