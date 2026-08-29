package rocco

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zoobz-io/capitan"
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
		lookupKey := strings.TrimSuffix(param, "...")
		if val := r.PathValue(lookupKey); val != "" {
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

// decodeRequestBody reads, decodes, and validates the request body according to
// the request contract. Shared by Handler and StreamHandler so the two paths
// cannot drift. On failure it writes the error response and returns the HTTP
// status with a non-nil error; callers return both unchanged. On success the
// error is nil and the status is 0.
func decodeRequestBody[In any](ctx context.Context, r *http.Request, w http.ResponseWriter, contract RequestContract, unmarshal func([]byte, any) error, validatable bool, handlerName string) (input In, status int, err error) {
	if contract.Kind == BodyNone || r.Body == nil {
		return input, 0, nil
	}

	// Limit body size if configured - use MaxBytesReader for proper 413 errors.
	if contract.MaxBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, contract.MaxBytes)
	}

	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		// Check if this is a max bytes exceeded error.
		var maxBytesErr *http.MaxBytesError
		if errors.As(readErr, &maxBytesErr) {
			capitan.Warn(ctx, RequestBodyReadError,
				HandlerNameKey.Field(handlerName),
				ErrorKey.Field("payload too large"),
			)
			writeError(ctx, w, ErrPayloadTooLarge.WithDetails(PayloadTooLargeDetails{
				MaxSize: contract.MaxBytes,
			}), handlerName)
			return input, http.StatusRequestEntityTooLarge, readErr
		}
		capitan.Error(ctx, RequestBodyReadError,
			HandlerNameKey.Field(handlerName),
			ErrorKey.Field(readErr.Error()),
		)
		writeError(ctx, w, ErrBadRequest.WithMessage("failed to read request body").WithCause(readErr), handlerName)
		return input, http.StatusBadRequest, readErr
	}
	if closeErr := r.Body.Close(); closeErr != nil {
		capitan.Warn(ctx, RequestBodyCloseError,
			HandlerNameKey.Field(handlerName),
			ErrorKey.Field(closeErr.Error()),
		)
	}

	// Raw contract: hand over the bytes and the incoming content type without
	// decoding. BodyRaw is only set when In is RawBody.
	if contract.Kind == BodyRaw {
		if rb, ok := any(&input).(*RawBody); ok {
			rb.ContentType = r.Header.Get("Content-Type")
			rb.Data = body
		}
		return input, 0, nil
	}

	if len(body) == 0 {
		return input, 0, nil
	}

	if unmarshalErr := unmarshal(body, &input); unmarshalErr != nil {
		capitan.Error(ctx, RequestBodyParseError,
			HandlerNameKey.Field(handlerName),
			ErrorKey.Field(unmarshalErr.Error()),
		)
		writeError(ctx, w, ErrUnprocessableEntity.WithMessage("invalid request body").WithCause(unmarshalErr), handlerName)
		return input, http.StatusUnprocessableEntity, unmarshalErr
	}

	// Validate input if type implements Validatable.
	if validatable {
		if v, ok := any(input).(Validatable); ok {
			if inputErr := v.Validate(); inputErr != nil {
				capitan.Warn(ctx, RequestValidationInputFailed,
					HandlerNameKey.Field(handlerName),
					ErrorKey.Field(inputErr.Error()),
				)
				writeValidationErrorResponse(ctx, w, inputErr, handlerName)
				return input, http.StatusUnprocessableEntity, inputErr
			}
		}
	}

	return input, 0, nil
}
