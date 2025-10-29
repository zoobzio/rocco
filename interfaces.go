package rocco

import (
	"context"
	"net/http"
)

// RouteHandler represents an HTTP route handler with metadata.
type RouteHandler interface {
	// Process handles the HTTP request and writes the response.
	// Returns the HTTP status code written and any error encountered.
	Process(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error)

	// Name returns the handler identifier
	Name() string

	// Routing metadata
	Method() string
	Path() string

	// Middleware
	Middleware() []func(http.Handler) http.Handler

	// Optional metadata for documentation
	Summary() string
	Description() string
	Tags() []string

	// OpenAPI generation metadata
	PathParams() []string
	QueryParams() []string
	SuccessStatus() int
	ErrorCodes() []int
	InputSchema() *Schema   // Returns OpenAPI schema for request body
	OutputSchema() *Schema  // Returns OpenAPI schema for response body
	InputTypeName() string  // Returns the Go type name for request body
	OutputTypeName() string // Returns the Go type name for response body

	// Lifecycle
	Close() error
}
