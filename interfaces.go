package rocco

import (
	"context"
	"net/http"
)

// Endpoint represents an HTTP route handler with metadata.
type Endpoint interface {
	// Process handles the HTTP request and writes the response.
	// Returns the HTTP status code written and any error encountered.
	Process(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error)

	// Spec returns the declarative specification for this handler
	Spec() HandlerSpec

	// Middleware returns handler-specific middleware
	Middleware() []func(http.Handler) http.Handler

	// Lifecycle
	Close() error
}
