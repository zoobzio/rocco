package rocco

import (
	"context"
	"fmt"
	"net/http"
)

// noBodyTypeName is the sentinel type name for handlers without a request body.
const noBodyTypeName = "NoBody"

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

// extractParams extracts and validates required parameters from the request.
func extractParams(_ context.Context, r *http.Request, pathParams, queryParams []string) (*Params, error) {
	params := &Params{
		Path:  make(map[string]string),
		Query: make(map[string]string),
	}

	// Extract path params using Go 1.22+ PathValue.
	for _, param := range pathParams {
		if val := r.PathValue(param); val != "" {
			params.Path[param] = val
		} else {
			return nil, fmt.Errorf("path parameter %q", param)
		}
	}

	// Extract only declared query params.
	if len(queryParams) > 0 {
		query := r.URL.Query()
		for _, declaredParam := range queryParams {
			if values := query[declaredParam]; len(values) > 0 {
				params.Query[declaredParam] = values[0]
			}
		}
	}

	return params, nil
}
