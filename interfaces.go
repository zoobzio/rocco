package rocco

import (
	"context"
	"net/http"

	"github.com/zoobzio/metricz"
	"github.com/zoobzio/tracez"
)

// RouteHandler represents an HTTP route handler with metadata.
type RouteHandler interface {
	// Process handles the HTTP request and writes the response
	Process(ctx context.Context, r *http.Request, w http.ResponseWriter) error

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

	// Observability
	Metrics() *metricz.Registry
	Tracer() *tracez.Tracer

	// Lifecycle
	Close() error
}
