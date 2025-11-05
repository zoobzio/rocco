package rocco

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/zoobzio/capitan"
	"github.com/zoobzio/sentinel"
)

// UsageLimit represents a usage limit check with a dynamic threshold callback.
type UsageLimit struct {
	Key           string             // Stats key to check (e.g., "requests_today")
	ThresholdFunc func(Identity) int // Function that returns threshold for this identity
}

// Handler wraps a typed handler function with metadata for documentation and parsing.
// It implements Endpoint interface.
// The handler function receives a Request with typed input and parameters.
type Handler[In, Out any] struct {
	// Core handler function receives Request with typed body.
	fn func(*Request[In]) (Out, error)

	// Declarative specification
	spec HandlerSpec

	// Runtime configuration
	responseHeaders map[string]string // Default response headers.
	maxBodySize     int64             // Maximum request body size in bytes (0 = unlimited, default: 10MB).

	// Type metadata from sentinel.
	InputMeta  sentinel.ModelMetadata
	OutputMeta sentinel.ModelMetadata

	// Validation.
	validator *validator.Validate

	// Middleware.
	middleware []func(http.Handler) http.Handler
}

// Process implements Endpoint.
func (h *Handler[In, Out]) Process(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error) {
	// Emit handler executing event
	capitan.Debug(ctx, HandlerExecuting,
		HandlerNameKey.Field(h.spec.Name),
	)

	// Extract and validate parameters.
	params, err := h.extractParams(ctx, r)
	if err != nil {
		capitan.Error(ctx, RequestParamsInvalid,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeErrorResponse(w, http.StatusUnprocessableEntity)
		return http.StatusUnprocessableEntity, err
	}

	// Parse request body.
	var input In
	if h.InputMeta.TypeName != "NoBody" && r.Body != nil {
		// Limit body size if configured
		var bodyReader io.Reader = r.Body
		if h.maxBodySize > 0 {
			bodyReader = io.LimitReader(r.Body, h.maxBodySize)
		}

		body, readErr := io.ReadAll(bodyReader)
		if readErr != nil {
			capitan.Error(ctx, RequestBodyReadError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(readErr.Error()),
			)
			writeErrorResponse(w, http.StatusBadRequest)
			return http.StatusBadRequest, readErr
		}
		r.Body.Close()

		if len(body) > 0 {
			if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
				capitan.Error(ctx, RequestBodyParseError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(unmarshalErr.Error()),
				)
				writeErrorResponse(w, http.StatusUnprocessableEntity)
				return http.StatusUnprocessableEntity, unmarshalErr
			}

			// Validate input.
			if inputErr := h.validator.Struct(input); inputErr != nil {
				capitan.Warn(ctx, RequestValidationInputFailed,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(inputErr.Error()),
				)
				writeValidationErrorResponse(w, inputErr)
				return http.StatusUnprocessableEntity, inputErr
			}
		}
	}

	// Extract identity from context if present
	var identity Identity = NoIdentity{}
	if val := ctx.Value(identityContextKey); val != nil {
		if id, ok := val.(Identity); ok {
			identity = id
		}
	}

	// Create Request for callback.
	req := &Request[In]{
		Context:  ctx,
		Request:  r,
		Params:   params,
		Body:     input,
		Identity: identity,
	}

	// Call user handler.
	output, err := h.fn(req)
	if err != nil {
		// Check if this is a sentinel error.
		if isSentinelError(err) {
			status := mapSentinelToStatus(err)

			// Validate that this error code is declared.
			if !h.isErrorCodeDeclared(status) {
				// Undeclared sentinel error - programming error.
				capitan.Warn(ctx, HandlerUndeclaredSentinel,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(err.Error()),
					StatusCodeKey.Field(status),
				)
				writeErrorResponse(w, http.StatusInternalServerError)
				return http.StatusInternalServerError, fmt.Errorf("undeclared sentinel error %w (add %d to WithErrorCodes)", err, status)
			}

			// Declared sentinel error - successful handling.
			capitan.Warn(ctx, HandlerSentinelError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(err.Error()),
				StatusCodeKey.Field(status),
			)
			writeErrorResponse(w, status)
			return status, nil
		}

		// Real error.
		capitan.Error(ctx, HandlerError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeErrorResponse(w, http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}

	// Validate output.
	if validErr := h.validator.Struct(output); validErr != nil {
		capitan.Warn(ctx, RequestValidationOutputFailed,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(validErr.Error()),
		)
		writeErrorResponse(w, http.StatusInternalServerError)
		return http.StatusInternalServerError, fmt.Errorf("output validation failed: %w", validErr)
	}

	// Marshal response.
	body, err := json.Marshal(output)
	if err != nil {
		capitan.Error(ctx, RequestResponseMarshalError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeErrorResponse(w, http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}

	// Write response headers.
	for key, value := range h.responseHeaders {
		w.Header().Set(key, value)
	}
	w.Header().Set("Content-Type", "application/json")

	// Write status and body.
	w.WriteHeader(h.spec.SuccessStatus)
	w.Write(body)

	// Emit handler success event
	capitan.Info(ctx, HandlerSuccess,
		HandlerNameKey.Field(h.spec.Name),
		StatusCodeKey.Field(h.spec.SuccessStatus),
	)

	return h.spec.SuccessStatus, nil
}

// Spec implements Endpoint.
func (h *Handler[In, Out]) Spec() HandlerSpec {
	return h.spec
}

// Close implements Endpoint.
func (*Handler[In, Out]) Close() error {
	return nil
}

// NewHandler creates a new typed handler with sentinel metadata.
func NewHandler[In, Out any](name string, method, path string, fn func(*Request[In]) (Out, error)) *Handler[In, Out] {
	inputMeta := sentinel.Scan[In]()
	outputMeta := sentinel.Scan[Out]()

	return &Handler[In, Out]{
		fn: fn,
		spec: HandlerSpec{
			Name:           name,
			Method:         method,
			Path:           path,
			PathParams:     []string{},
			QueryParams:    []string{},
			InputTypeName:  inputMeta.TypeName,
			OutputTypeName: outputMeta.TypeName,
			SuccessStatus:  http.StatusOK, // Default to 200.
			ErrorCodes:     []int{},
			RequiresAuth:   false,
			ScopeGroups:    [][]string{},
			RoleGroups:     [][]string{},
			UsageLimits:    []UsageLimit{},
			Tags:           []string{},
		},
		responseHeaders: make(map[string]string),
		maxBodySize:     10 * 1024 * 1024, // Default to 10MB.
		InputMeta:       inputMeta,
		OutputMeta:      outputMeta,
		validator:       validator.New(),
		middleware:      make([]func(http.Handler) http.Handler, 0),
	}
}

// WithSummary sets the OpenAPI summary.
func (h *Handler[In, Out]) WithSummary(summary string) *Handler[In, Out] {
	h.spec.Summary = summary
	return h
}

// WithDescription sets the OpenAPI description.
func (h *Handler[In, Out]) WithDescription(desc string) *Handler[In, Out] {
	h.spec.Description = desc
	return h
}

// WithTags sets the OpenAPI tags.
func (h *Handler[In, Out]) WithTags(tags ...string) *Handler[In, Out] {
	h.spec.Tags = tags
	return h
}

// WithSuccessStatus sets the HTTP status code for successful responses.
func (h *Handler[In, Out]) WithSuccessStatus(status int) *Handler[In, Out] {
	h.spec.SuccessStatus = status
	return h
}

// WithPathParams specifies required path parameters.
func (h *Handler[In, Out]) WithPathParams(params ...string) *Handler[In, Out] {
	h.spec.PathParams = params
	return h
}

// WithQueryParams specifies required query parameters.
func (h *Handler[In, Out]) WithQueryParams(params ...string) *Handler[In, Out] {
	h.spec.QueryParams = params
	return h
}

// WithResponseHeaders sets default response headers for this handler.
func (h *Handler[In, Out]) WithResponseHeaders(headers map[string]string) *Handler[In, Out] {
	h.responseHeaders = headers
	return h
}

// WithErrorCodes declares which HTTP error status codes this handler may return.
// Undeclared sentinel errors will be converted to 500 Internal Server Error.
// This is used for OpenAPI documentation generation.
func (h *Handler[In, Out]) WithErrorCodes(codes ...int) *Handler[In, Out] {
	h.spec.ErrorCodes = codes
	return h
}

// WithMaxBodySize sets the maximum request body size in bytes for this handler.
// Set to 0 for unlimited (not recommended for production).
func (h *Handler[In, Out]) WithMaxBodySize(size int64) *Handler[In, Out] {
	h.maxBodySize = size
	return h
}

// WithMiddleware adds middleware to this handler and returns the handler for chaining.
func (h *Handler[In, Out]) WithMiddleware(middleware ...func(http.Handler) http.Handler) *Handler[In, Out] {
	h.middleware = append(h.middleware, middleware...)
	return h
}

// Middleware implements Endpoint.
func (h *Handler[In, Out]) Middleware() []func(http.Handler) http.Handler {
	return h.middleware
}

// WithAuthentication marks this handler as requiring authentication.
func (h *Handler[In, Out]) WithAuthentication() *Handler[In, Out] {
	h.spec.RequiresAuth = true
	return h
}

// WithScopes adds a scope requirement group (OR logic within group, AND across multiple calls).
// Example: .WithScopes("read", "write") requires (read OR write).
// Calling multiple times creates AND: .WithScopes("read").WithScopes("admin") = read AND admin.
func (h *Handler[In, Out]) WithScopes(scopes ...string) *Handler[In, Out] {
	if len(scopes) > 0 {
		h.spec.ScopeGroups = append(h.spec.ScopeGroups, scopes)
		// Scopes require authentication
		h.spec.RequiresAuth = true
	}
	return h
}

// WithRoles adds a role requirement group (OR logic within group, AND across multiple calls).
// Example: .WithRoles("admin", "moderator") requires (admin OR moderator).
// Calling multiple times creates AND: .WithRoles("admin").WithRoles("verified") = admin AND verified.
func (h *Handler[In, Out]) WithRoles(roles ...string) *Handler[In, Out] {
	if len(roles) > 0 {
		h.spec.RoleGroups = append(h.spec.RoleGroups, roles)
		// Roles require authentication
		h.spec.RequiresAuth = true
	}
	return h
}

// WithUsageLimit adds a usage limit check based on identity stats.
// The handler will return 429 Too Many Requests if identity.Stats()[key] >= thresholdFunc(identity).
// The thresholdFunc is called with the identity to allow dynamic limits per user/tenant.
// Usage limits require authentication.
func (h *Handler[In, Out]) WithUsageLimit(key string, thresholdFunc func(Identity) int) *Handler[In, Out] {
	h.spec.UsageLimits = append(h.spec.UsageLimits, UsageLimit{
		Key:           key,
		ThresholdFunc: thresholdFunc,
	})
	// Usage limits require authentication
	h.spec.RequiresAuth = true
	return h
}

// extractParams extracts and validates required parameters from the request.
func (h *Handler[In, Out]) extractParams(ctx context.Context, r *http.Request) (*Params, error) {
	params := &Params{
		Path:  make(map[string]string),
		Query: make(map[string]string),
	}

	// Extract path params from Chi router context.
	if rctx := chi.RouteContext(ctx); rctx != nil {
		for i, key := range rctx.URLParams.Keys {
			params.Path[key] = rctx.URLParams.Values[i]
		}
	}

	// Validate required path params.
	for _, requiredParam := range h.spec.PathParams {
		if _, exists := params.Path[requiredParam]; !exists {
			return nil, fmt.Errorf("path parameter %q", requiredParam)
		}
	}

	// Extract only declared query params (if any declared).
	if len(h.spec.QueryParams) > 0 {
		query := r.URL.Query()
		for _, declaredParam := range h.spec.QueryParams {
			if values := query[declaredParam]; len(values) > 0 {
				params.Query[declaredParam] = values[0]
			}
			// Missing query params result in empty string (not an error).
		}
	}
	// If no query params declared, Query map stays empty.

	return params, nil
}

// isSentinelError checks if an error is one of our sentinel errors.
// that indicate specific HTTP error status codes.
func isSentinelError(err error) bool {
	return errors.Is(err, ErrBadRequest) ||
		errors.Is(err, ErrUnauthorized) ||
		errors.Is(err, ErrForbidden) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrUnprocessableEntity) ||
		errors.Is(err, ErrTooManyRequests)
}

// errorResponse represents the standard error response format.
//
//nolint:unused // Used indirectly via reflection in JSON marshaling
type errorResponse struct {
	Error string `json:"error"`
}

// Canned error responses - consistent across all handlers.
var cannedErrorResponses = map[int][]byte{
	http.StatusBadRequest:          []byte(`{"error":"Bad Request"}`),
	http.StatusUnauthorized:        []byte(`{"error":"Unauthorized"}`),
	http.StatusForbidden:           []byte(`{"error":"Forbidden"}`),
	http.StatusNotFound:            []byte(`{"error":"Not Found"}`),
	http.StatusConflict:            []byte(`{"error":"Conflict"}`),
	http.StatusUnprocessableEntity: []byte(`{"error":"Unprocessable Entity"}`),
	http.StatusTooManyRequests:     []byte(`{"error":"Too Many Requests"}`),
	http.StatusInternalServerError: []byte(`{"error":"Internal Server Error"}`),
}

// mapSentinelToStatus maps sentinel errors to HTTP status codes.
func mapSentinelToStatus(err error) int {
	switch {
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrUnprocessableEntity):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrTooManyRequests):
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// isErrorCodeDeclared checks if an error status code was declared via WithErrorCodes.
func (h *Handler[In, Out]) isErrorCodeDeclared(status int) bool {
	for _, code := range h.spec.ErrorCodes {
		if code == status {
			return true
		}
	}
	return false
}

// writeErrorResponse writes a canned JSON error response.
func writeErrorResponse(w http.ResponseWriter, status int) {
	body, exists := cannedErrorResponses[status]
	if !exists {
		body = cannedErrorResponses[http.StatusInternalServerError]
		status = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// writeValidationErrorResponse writes detailed validation errors.
func writeValidationErrorResponse(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)

	// Extract validation errors.
	var validationErrors []map[string]string
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		for _, fe := range ve {
			validationErrors = append(validationErrors, map[string]string{
				"field": fe.Field(),
				"tag":   fe.Tag(),
				"value": fmt.Sprintf("%v", fe.Value()),
			})
		}
	}

	response := map[string]any{
		"error":  "Validation failed",
		"fields": validationErrors,
	}

	//nolint:errchkjson // Standard practice after WriteHeader
	json.NewEncoder(w).Encode(response)
}
