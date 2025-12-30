package rocco

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
	validateOutput  bool              // Whether to validate output structs (disabled by default).

	// Type metadata from sentinel.
	InputMeta  sentinel.ModelMetadata
	OutputMeta sentinel.ModelMetadata

	// Error definitions with schemas for OpenAPI generation.
	errorDefs []ErrorDefinition

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
	params, err := extractParams(ctx, r, h.spec.PathParams, h.spec.QueryParams)
	if err != nil {
		capitan.Error(ctx, RequestParamsInvalid,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeError(w, ErrUnprocessableEntity.WithMessage("invalid parameters").WithCause(err))
		return http.StatusUnprocessableEntity, err
	}

	// Parse request body.
	var input In
	if h.InputMeta.TypeName != noBodyTypeName && r.Body != nil {
		// Limit body size if configured - use MaxBytesReader for proper 413 errors
		if h.maxBodySize > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
		}

		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			// Check if this is a max bytes exceeded error
			var maxBytesErr *http.MaxBytesError
			if errors.As(readErr, &maxBytesErr) {
				capitan.Warn(ctx, RequestBodyReadError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field("payload too large"),
				)
				writeError(w, ErrPayloadTooLarge.WithDetails(PayloadTooLargeDetails{
					MaxSize: h.maxBodySize,
				}))
				return http.StatusRequestEntityTooLarge, readErr
			}
			capitan.Error(ctx, RequestBodyReadError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(readErr.Error()),
			)
			writeError(w, ErrBadRequest.WithMessage("failed to read request body").WithCause(readErr))
			return http.StatusBadRequest, readErr
		}
		r.Body.Close()

		if len(body) > 0 {
			if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
				capitan.Error(ctx, RequestBodyParseError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(unmarshalErr.Error()),
				)
				writeError(w, ErrUnprocessableEntity.WithMessage("invalid request body").WithCause(unmarshalErr))
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
		// Check if this is a rocco Error.
		if e := getRoccoError(err); e != nil {
			// Validate that this error is declared.
			if !h.isErrorDeclared(e) {
				// Undeclared error - programming error.
				capitan.Warn(ctx, HandlerUndeclaredSentinel,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(err.Error()),
					StatusCodeKey.Field(e.Status()),
				)
				writeError(w, ErrInternalServer)
				return http.StatusInternalServerError, fmt.Errorf("undeclared error %s (add to WithErrors)", e.Code())
			}

			// Declared error - successful handling.
			capitan.Warn(ctx, HandlerSentinelError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(err.Error()),
				StatusCodeKey.Field(e.Status()),
			)
			writeError(w, e)
			return e.Status(), nil
		}

		// Real unexpected error.
		capitan.Error(ctx, HandlerError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeError(w, ErrInternalServer)
		return http.StatusInternalServerError, err
	}

	// Validate output (opt-in, disabled by default).
	if h.validateOutput {
		if validErr := h.validator.Struct(output); validErr != nil {
			capitan.Warn(ctx, RequestValidationOutputFailed,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(validErr.Error()),
			)
			writeError(w, ErrInternalServer.WithCause(fmt.Errorf("output validation failed: %w", validErr)))
			return http.StatusInternalServerError, fmt.Errorf("output validation failed: %w", validErr)
		}
	}

	// Marshal response.
	body, err := json.Marshal(output)
	if err != nil {
		capitan.Error(ctx, RequestResponseMarshalError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeError(w, ErrInternalServer.WithCause(err))
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

// WithErrors declares which errors this handler may return.
// Undeclared errors will be converted to 500 Internal Server Error.
// This is used for OpenAPI documentation generation with proper error schemas.
func (h *Handler[In, Out]) WithErrors(errs ...ErrorDefinition) *Handler[In, Out] {
	h.errorDefs = append(h.errorDefs, errs...)
	// Also populate ErrorCodes for spec serialization
	for _, err := range errs {
		h.spec.ErrorCodes = append(h.spec.ErrorCodes, err.Status())
	}
	return h
}

// ErrorDefs returns the declared error definitions for this handler.
// Used by OpenAPI generation to extract error schemas.
func (h *Handler[In, Out]) ErrorDefs() []ErrorDefinition {
	return h.errorDefs
}

// WithMaxBodySize sets the maximum request body size in bytes for this handler.
// Set to 0 for unlimited (not recommended for production).
func (h *Handler[In, Out]) WithMaxBodySize(size int64) *Handler[In, Out] {
	h.maxBodySize = size
	return h
}

// WithOutputValidation enables validation of output structs before sending responses.
// This is disabled by default for performance. Enable in development to catch bugs early.
// Output validation failures return 500 Internal Server Error.
func (h *Handler[In, Out]) WithOutputValidation() *Handler[In, Out] {
	h.validateOutput = true
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

// getRoccoError extracts a rocco ErrorDefinition from an error chain.
// Returns nil if the error is not a rocco Error.
func getRoccoError(err error) ErrorDefinition {
	var e ErrorDefinition
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// errorResponse represents the standard error response format.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// isErrorDeclared checks if an error was declared via WithErrors.
// Matches by error code (e.g., "NOT_FOUND"), not just status code.
func (h *Handler[In, Out]) isErrorDeclared(err ErrorDefinition) bool {
	for _, declared := range h.errorDefs {
		if declared.Code() == err.Code() {
			return true
		}
	}
	return false
}

// writeError writes a structured JSON error response.
func writeError(w http.ResponseWriter, err ErrorDefinition) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status())

	//nolint:errchkjson // Standard practice after WriteHeader
	json.NewEncoder(w).Encode(errorResponse{
		Code:    err.Code(),
		Message: err.Message(),
		Details: err.DetailsAny(),
	})
}

// writeValidationErrorResponse writes detailed validation errors using the standard error format.
func writeValidationErrorResponse(w http.ResponseWriter, err error) {
	// Extract validation errors.
	var validationErrors []ValidationFieldError
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		for _, fe := range ve {
			validationErrors = append(validationErrors, ValidationFieldError{
				Field: fe.Field(),
				Tag:   fe.Tag(),
				Value: fmt.Sprintf("%v", fe.Value()),
			})
		}
	}

	writeError(w, ErrValidationFailed.WithDetails(ValidationDetails{
		Fields: validationErrors,
	}))
}
