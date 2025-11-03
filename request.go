package rocco

import (
	"context"
	"net/http"
)

// Request holds all data needed by handler callbacks.
// It embeds context and the underlying HTTP request for full access.
//
// IMPORTANT: Modifying the embedded *http.Request (headers, etc.) is not
// recommended as changes won't be reflected in OpenAPI documentation or
// handler configuration. Use Handler builder methods (WithResponseHeaders,
// WithSuccessStatus) for documented behavior.
type Request[In any] struct {
	context.Context // Embedded for deadline, cancellation, values
	*http.Request   // Embedded for direct access when needed (use sparingly)
	Params          *Params
	Body            In
	Identity        Identity // Authenticated identity (nil/NoIdentity for public endpoints)
}

// Params holds extracted request parameters.
type Params struct {
	Path  map[string]string // Path parameters (e.g., /users/{id})
	Query map[string]string // Query parameters (e.g., ?page=1)
}

// NoBody represents an empty input for handlers that don't expect a request body.
// Used for GET, HEAD, DELETE requests.
type NoBody struct{}
