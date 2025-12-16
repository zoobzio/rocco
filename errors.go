package rocco

import (
	"github.com/zoobzio/sentinel"
)

// ErrorDefinition is the interface that all error definitions satisfy.
// It provides non-generic access to error metadata for the engine and OpenAPI generation.
type ErrorDefinition interface {
	error
	Code() string
	Status() int
	Message() string
	DetailsAny() any                     // For serialization (type-erased)
	DetailsMeta() sentinel.ModelMetadata // For OpenAPI schema generation
}

// Error represents a structured API error with code, status, message, and typed details.
// The type parameter D captures the details type at compile time for type-safe usage
// and OpenAPI schema generation.
type Error[D any] struct {
	code        string
	status      int
	message     string
	details     D
	cause       error
	detailsMeta sentinel.ModelMetadata
}

// Error implements the error interface.
func (e *Error[D]) Error() string {
	return e.message
}

// Code returns the error code (e.g., "NOT_FOUND").
func (e *Error[D]) Code() string {
	return e.code
}

// Status returns the HTTP status code.
func (e *Error[D]) Status() int {
	return e.status
}

// Message returns the human-readable error message.
func (e *Error[D]) Message() string {
	return e.message
}

// Details returns the typed error details.
func (e *Error[D]) Details() D {
	return e.details
}

// DetailsAny returns the error details as any for serialization.
// This satisfies the ErrorDefinition interface.
func (e *Error[D]) DetailsAny() any {
	return e.details
}

// DetailsMeta returns the sentinel metadata for the details type.
// Used by OpenAPI generation to create schemas.
func (e *Error[D]) DetailsMeta() sentinel.ModelMetadata {
	return e.detailsMeta
}

// Unwrap returns the underlying cause error for error chain traversal.
func (e *Error[D]) Unwrap() error {
	return e.cause
}

// Is enables errors.Is() to match against base error codes.
// Two errors match if they have the same code.
func (e *Error[D]) Is(target error) bool {
	if t, ok := target.(ErrorDefinition); ok {
		return e.code == t.Code()
	}
	return false
}

// NewError creates a new error definition with the given code, status, and message.
// The type parameter D specifies the details struct type for this error.
// Use this to define custom error types with typed details.
func NewError[D any](code string, status int, message string) *Error[D] {
	return &Error[D]{
		code:        code,
		status:      status,
		message:     message,
		detailsMeta: sentinel.Scan[D](),
	}
}

// WithMessage returns a new error with a custom message.
// The original error is not modified (immutable).
func (e *Error[D]) WithMessage(message string) *Error[D] {
	return &Error[D]{
		code:        e.code,
		status:      e.status,
		message:     message,
		details:     e.details,
		cause:       e.cause,
		detailsMeta: e.detailsMeta,
	}
}

// WithDetails returns a new error with the given typed details.
// The original error is not modified (immutable).
func (e *Error[D]) WithDetails(details D) *Error[D] {
	return &Error[D]{
		code:        e.code,
		status:      e.status,
		message:     e.message,
		details:     details,
		cause:       e.cause,
		detailsMeta: e.detailsMeta,
	}
}

// WithCause returns a new error with the given underlying cause.
// The cause is not exposed to API clients but is available for logging/debugging.
// The original error is not modified (immutable).
func (e *Error[D]) WithCause(cause error) *Error[D] {
	return &Error[D]{
		code:        e.code,
		status:      e.status,
		message:     e.message,
		details:     e.details,
		cause:       cause,
		detailsMeta: e.detailsMeta,
	}
}

// NoDetails is used for errors that don't carry additional details.
type NoDetails struct{}

// Built-in error details types

// BadRequestDetails provides context for bad request errors.
type BadRequestDetails struct {
	Reason string `json:"reason,omitempty" description:"Why the request was invalid"`
}

// UnauthorizedDetails provides context for authentication errors.
type UnauthorizedDetails struct {
	Reason string `json:"reason,omitempty" description:"Why authentication failed"`
}

// ForbiddenDetails provides context for authorization errors.
type ForbiddenDetails struct {
	Reason string `json:"reason,omitempty" description:"Why access was denied"`
}

// NotFoundDetails provides context for not found errors.
type NotFoundDetails struct {
	Resource string `json:"resource,omitempty" description:"The type of resource that was not found"`
}

// ConflictDetails provides context for conflict errors.
type ConflictDetails struct {
	Reason string `json:"reason,omitempty" description:"What caused the conflict"`
}

// UnprocessableEntityDetails provides context for validation errors.
type UnprocessableEntityDetails struct {
	Reason string `json:"reason,omitempty" description:"Why the entity was unprocessable"`
}

// ValidationFieldError represents a single field validation error.
type ValidationFieldError struct {
	Field string `json:"field" description:"The field that failed validation"`
	Tag   string `json:"tag" description:"The validation rule that failed"`
	Value string `json:"value" description:"The value that failed validation"`
}

// ValidationDetails provides detailed validation errors for request validation.
type ValidationDetails struct {
	Fields []ValidationFieldError `json:"fields" description:"List of validation errors"`
}

// PayloadTooLargeDetails provides context for payload size errors.
type PayloadTooLargeDetails struct {
	MaxSize int64 `json:"max_size,omitempty" description:"Maximum allowed payload size in bytes"`
}

// TooManyRequestsDetails provides context for rate limit errors.
type TooManyRequestsDetails struct {
	RetryAfter int `json:"retry_after,omitempty" description:"Seconds until the client can retry"`
}

// InternalServerDetails provides context for internal errors (not exposed to clients).
type InternalServerDetails struct {
	Reason string `json:"reason,omitempty" description:"Internal error context"`
}

// NotImplementedDetails provides context for not implemented errors.
type NotImplementedDetails struct {
	Feature string `json:"feature,omitempty" description:"The feature that is not implemented"`
}

// ServiceUnavailableDetails provides context for service unavailable errors.
type ServiceUnavailableDetails struct {
	Reason string `json:"reason,omitempty" description:"Why the service is unavailable"`
}

// Client errors (4xx)
var (
	// ErrBadRequest indicates the request was invalid (400)
	ErrBadRequest = NewError[BadRequestDetails]("BAD_REQUEST", 400, "bad request")

	// ErrUnauthorized indicates missing or invalid authentication (401)
	ErrUnauthorized = NewError[UnauthorizedDetails]("UNAUTHORIZED", 401, "unauthorized")

	// ErrForbidden indicates the request is not allowed (403)
	ErrForbidden = NewError[ForbiddenDetails]("FORBIDDEN", 403, "forbidden")

	// ErrNotFound indicates the resource was not found (404)
	ErrNotFound = NewError[NotFoundDetails]("NOT_FOUND", 404, "not found")

	// ErrConflict indicates a conflict with existing data (409)
	ErrConflict = NewError[ConflictDetails]("CONFLICT", 409, "conflict")

	// ErrPayloadTooLarge indicates the request body exceeds the size limit (413)
	ErrPayloadTooLarge = NewError[PayloadTooLargeDetails]("PAYLOAD_TOO_LARGE", 413, "payload too large")

	// ErrUnprocessableEntity indicates the request was well-formed but semantically invalid (422)
	ErrUnprocessableEntity = NewError[UnprocessableEntityDetails]("UNPROCESSABLE_ENTITY", 422, "unprocessable entity")

	// ErrValidationFailed indicates request validation failed with detailed field errors (422)
	ErrValidationFailed = NewError[ValidationDetails]("VALIDATION_FAILED", 422, "validation failed")

	// ErrTooManyRequests indicates rate limiting (429)
	ErrTooManyRequests = NewError[TooManyRequestsDetails]("TOO_MANY_REQUESTS", 429, "too many requests")
)

// Server errors (5xx)
var (
	// ErrInternalServer indicates an unexpected server error (500)
	ErrInternalServer = NewError[InternalServerDetails]("INTERNAL_SERVER_ERROR", 500, "internal server error")

	// ErrNotImplemented indicates the functionality is not implemented (501)
	ErrNotImplemented = NewError[NotImplementedDetails]("NOT_IMPLEMENTED", 501, "not implemented")

	// ErrServiceUnavailable indicates the service is temporarily unavailable (503)
	ErrServiceUnavailable = NewError[ServiceUnavailableDetails]("SERVICE_UNAVAILABLE", 503, "service unavailable")
)
