package rocco

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zoobz-io/capitan"
	"github.com/zoobz-io/check"
	"github.com/zoobz-io/sentinel"
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
	codec           Codec             // Codec for request/response serialization.
	codecExplicit   bool              // True if codec was explicitly set via WithCodec.

	// Type metadata from sentinel.
	InputMeta  sentinel.Metadata
	OutputMeta sentinel.Metadata

	// Error definitions with schemas for OpenAPI generation, keyed by error code.
	errorDefs map[string]ErrorDefinition

	// Validation flags (checked once at creation time).
	inputValidatable  bool // True if In implements Validatable.
	outputValidatable bool // True if Out implements Validatable.

	// Middleware.
	middleware []func(http.Handler) http.Handler

	// Hooks.
	inputEntryable bool // True if In implements Entryable.
	outputSendable bool // True if Out implements Sendable.
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
		writeError(ctx, w, ErrUnprocessableEntity.WithMessage("invalid parameters").WithCause(err), h.spec.ContentType, h.spec.Name)
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
				writeError(ctx, w, ErrPayloadTooLarge.WithDetails(PayloadTooLargeDetails{
					MaxSize: h.maxBodySize,
				}), h.spec.ContentType, h.spec.Name)
				return http.StatusRequestEntityTooLarge, readErr
			}
			capitan.Error(ctx, RequestBodyReadError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(readErr.Error()),
			)
			writeError(ctx, w, ErrBadRequest.WithMessage("failed to read request body").WithCause(readErr), h.spec.ContentType, h.spec.Name)
			return http.StatusBadRequest, readErr
		}
		if closeErr := r.Body.Close(); closeErr != nil {
			capitan.Warn(ctx, RequestBodyCloseError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(closeErr.Error()),
			)
		}

		if len(body) > 0 {
			if unmarshalErr := h.codec.Unmarshal(body, &input); unmarshalErr != nil {
				capitan.Error(ctx, RequestBodyParseError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(unmarshalErr.Error()),
				)
				writeError(ctx, w, ErrUnprocessableEntity.WithMessage("invalid request body").WithCause(unmarshalErr), h.spec.ContentType, h.spec.Name)
				return http.StatusUnprocessableEntity, unmarshalErr
			}

			// Validate input if type implements Validatable.
			if h.inputValidatable {
				if v, ok := any(input).(Validatable); ok {
					if inputErr := v.Validate(); inputErr != nil {
						capitan.Warn(ctx, RequestValidationInputFailed,
							HandlerNameKey.Field(h.spec.Name),
							ErrorKey.Field(inputErr.Error()),
						)
						writeValidationErrorResponse(ctx, w, inputErr, h.spec.ContentType, h.spec.Name)
						return http.StatusUnprocessableEntity, inputErr
					}
				}
			}
		}
	}

	// Run entry hook.
	if h.inputEntryable {
		if e, ok := any(&input).(Entryable); ok {
			if err = e.OnEntry(ctx); err != nil {
				capitan.Error(ctx, HandlerError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(err.Error()),
				)
				writeError(ctx, w, ErrInternalServer.WithCause(err), h.spec.ContentType, h.spec.Name)
				return http.StatusInternalServerError, err
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
				writeError(ctx, w, ErrInternalServer, h.spec.ContentType, h.spec.Name)
				return http.StatusInternalServerError, fmt.Errorf("undeclared error %s (add to WithErrors)", e.Code())
			}

			// Declared error - successful handling.
			capitan.Warn(ctx, HandlerSentinelError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(err.Error()),
				StatusCodeKey.Field(e.Status()),
			)
			writeError(ctx, w, e, h.spec.ContentType, h.spec.Name)
			return e.Status(), nil
		}

		// Real unexpected error.
		capitan.Error(ctx, HandlerError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeError(ctx, w, ErrInternalServer, h.spec.ContentType, h.spec.Name)
		return http.StatusInternalServerError, err
	}

	// Run send hook.
	if h.outputSendable {
		if e, ok := any(&output).(Sendable); ok {
			if err = e.OnSend(ctx); err != nil {
				capitan.Error(ctx, HandlerError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(err.Error()),
				)
				writeError(ctx, w, ErrInternalServer.WithCause(err), h.spec.ContentType, h.spec.Name)
				return http.StatusInternalServerError, err
			}
		}
	}

	// Check for redirect response.
	if redirect, ok := any(output).(Redirect); ok {
		// Guard against empty URL.
		if redirect.URL == "" {
			capitan.Error(ctx, HandlerError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field("redirect URL is empty"),
			)
			writeError(ctx, w, ErrInternalServer.WithMessage("redirect URL is empty"), h.spec.ContentType, h.spec.Name)
			return http.StatusInternalServerError, nil
		}

		status := redirect.Status
		if status == 0 {
			status = DefaultRedirectStatus
		}

		// Write custom response headers (e.g., cookies) but NOT Content-Type.
		for key, value := range h.responseHeaders {
			if key != "Content-Type" {
				w.Header().Set(key, value)
			}
		}

		// Write redirect-specific headers (e.g., Set-Cookie from session).
		for key, values := range redirect.Headers {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}

		// Write Location header and status.
		w.Header().Set("Location", redirect.URL)
		w.WriteHeader(status)

		// Emit success event.
		capitan.Info(ctx, HandlerSuccess,
			HandlerNameKey.Field(h.spec.Name),
			StatusCodeKey.Field(status),
		)

		return status, nil
	}

	// Validate output (opt-in, disabled by default).
	if h.validateOutput && h.outputValidatable {
		if v, ok := any(output).(Validatable); ok {
			if validErr := v.Validate(); validErr != nil {
				capitan.Warn(ctx, RequestValidationOutputFailed,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(validErr.Error()),
				)
				writeError(ctx, w, ErrInternalServer.WithCause(fmt.Errorf("output validation failed: %w", validErr)), h.spec.ContentType, h.spec.Name)
				return http.StatusInternalServerError, fmt.Errorf("output validation failed: %w", validErr)
			}
		}
	}

	// Marshal response.
	body, err := h.codec.Marshal(output)
	if err != nil {
		capitan.Error(ctx, RequestResponseMarshalError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeError(ctx, w, ErrInternalServer.WithCause(err), h.spec.ContentType, h.spec.Name)
		return http.StatusInternalServerError, err
	}

	// Write response headers.
	for key, value := range h.responseHeaders {
		w.Header().Set(key, value)
	}
	w.Header().Set("Content-Type", h.spec.ContentType)

	// Write status and body.
	w.WriteHeader(h.spec.SuccessStatus)
	if _, err := w.Write(body); err != nil {
		capitan.Warn(ctx, ResponseWriteError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
	}

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

	// Check if input/output types implement Validatable.
	var zeroIn In
	var zeroOut Out
	_, inputValidatable := any(zeroIn).(Validatable)
	_, outputValidatable := any(zeroOut).(Validatable)
	_, inputEntryable := any(&zeroIn).(Entryable)
	_, outputSendable := any(&zeroOut).(Sendable)

	return &Handler[In, Out]{
		fn: fn,
		spec: HandlerSpec{
			Name:           name,
			Method:         method,
			Path:           path,
			PathParams:     []string{},
			QueryParams:    []string{},
			InputTypeFQDN:  inputMeta.FQDN,
			InputTypeName:  inputMeta.TypeName,
			OutputTypeFQDN: outputMeta.FQDN,
			OutputTypeName: outputMeta.TypeName,
			SuccessStatus:  http.StatusOK, // Default to 200.
			ErrorCodes:     []int{},
			ContentType:    defaultCodec.ContentType(),
			RequiresAuth:   false,
			ScopeGroups:    [][]string{},
			RoleGroups:     [][]string{},
			UsageLimits:    []UsageLimit{},
			Tags:           []string{},
		},
		responseHeaders:   make(map[string]string),
		maxBodySize:       10 * 1024 * 1024, // Default to 10MB.
		codec:             defaultCodec,
		InputMeta:         inputMeta,
		OutputMeta:        outputMeta,
		errorDefs:         make(map[string]ErrorDefinition),
		inputValidatable:  inputValidatable,
		outputValidatable: outputValidatable,
		inputEntryable:    inputEntryable,
		outputSendable:    outputSendable,
		middleware:        make([]func(http.Handler) http.Handler, 0),
	}
}

// generateHandlerName creates a stable, unique handler name from method and path.
// Format: "method-path-segments-xxxxxxxx" where xxxxxxxx is an 8-char hex hash of
// the raw "method path" — deterministic across process restarts.
// Example: GET /users/{id} -> "get-users-id-a3f1b2c4".
//
// The name becomes the OpenAPI operationId and the handler name in log/error
// events, so it must not change between runs. The suffix is a short hash rather
// than the readable segments alone because two distinct routes can collapse to
// the same segments (e.g. "/users/{id}" and "/users/id" both yield "users-id");
// hashing the raw method+path keeps them distinct. Use WithName to override.
func generateHandlerName(method, path string) string {
	// Normalise method to lowercase
	name := strings.ToLower(method)

	// Process path segments
	trimmed := strings.Trim(path, "/")
	if trimmed != "" {
		segments := strings.Split(trimmed, "/")
		for _, seg := range segments {
			// Strip braces from path params: {id} -> id
			seg = strings.TrimPrefix(seg, "{")
			seg = strings.TrimSuffix(seg, "}")
			if seg != "" {
				name += "-" + seg
			}
		}
	}

	// Append 8 hex chars derived from a stable hash of the raw method+path.
	// Deterministic so operationIds and telemetry keys stay constant across
	// restarts; the raw path disambiguates segment collisions.
	sum := sha256.Sum256([]byte(method + " " + path))
	name += "-" + hex.EncodeToString(sum[:4])

	return name
}

// GET creates a handler for GET requests.
func GET[In, Out any](path string, fn func(*Request[In]) (Out, error)) *Handler[In, Out] {
	return NewHandler[In, Out](generateHandlerName(http.MethodGet, path), http.MethodGet, path, fn)
}

// POST creates a handler for POST requests.
func POST[In, Out any](path string, fn func(*Request[In]) (Out, error)) *Handler[In, Out] {
	return NewHandler[In, Out](generateHandlerName(http.MethodPost, path), http.MethodPost, path, fn)
}

// PUT creates a handler for PUT requests.
func PUT[In, Out any](path string, fn func(*Request[In]) (Out, error)) *Handler[In, Out] {
	return NewHandler[In, Out](generateHandlerName(http.MethodPut, path), http.MethodPut, path, fn)
}

// PATCH creates a handler for PATCH requests.
func PATCH[In, Out any](path string, fn func(*Request[In]) (Out, error)) *Handler[In, Out] {
	return NewHandler[In, Out](generateHandlerName(http.MethodPatch, path), http.MethodPatch, path, fn)
}

// DELETE creates a handler for DELETE requests.
func DELETE[In, Out any](path string, fn func(*Request[In]) (Out, error)) *Handler[In, Out] {
	return NewHandler[In, Out](generateHandlerName(http.MethodDelete, path), http.MethodDelete, path, fn)
}

// WithName sets a custom handler name, overriding the auto-generated one.
// This affects the OpenAPI operationId and log entries.
func (h *Handler[In, Out]) WithName(name string) *Handler[In, Out] {
	h.spec.Name = name
	return h
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
	for _, err := range errs {
		h.errorDefs[err.Code()] = err
	}
	// Rebuild ErrorCodes from deduplicated map
	h.spec.ErrorCodes = make([]int, 0, len(h.errorDefs))
	for _, err := range h.errorDefs {
		h.spec.ErrorCodes = append(h.spec.ErrorCodes, err.Status())
	}
	return h
}

// ErrorDefs returns the declared error definitions for this handler.
// Used by OpenAPI generation to extract error schemas.
func (h *Handler[In, Out]) ErrorDefs() []ErrorDefinition {
	defs := make([]ErrorDefinition, 0, len(h.errorDefs))
	for _, def := range h.errorDefs {
		defs = append(defs, def)
	}
	return defs
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

// WithCodec sets the codec for request/response serialization.
// This overrides the engine's default codec for this handler.
func (h *Handler[In, Out]) WithCodec(codec Codec) *Handler[In, Out] {
	h.codec = codec
	h.spec.ContentType = codec.ContentType()
	h.codecExplicit = true
	return h
}

// applyDefaultCodec sets the codec if one wasn't explicitly set via WithCodec.
// Called by Engine when registering handlers.
func (h *Handler[In, Out]) applyDefaultCodec(codec Codec) {
	if !h.codecExplicit {
		h.codec = codec
		h.spec.ContentType = codec.ContentType()
	}
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
	_, exists := h.errorDefs[err.Code()]
	return exists
}

// writeError writes a structured error response using the specified content type.
// Error responses are always JSON-encoded regardless of handler codec, as error
// schemas are defined in JSON format in OpenAPI.
func writeError(ctx context.Context, w http.ResponseWriter, err ErrorDefinition, contentType, handlerName string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(err.Status())

	if encodeErr := json.NewEncoder(w).Encode(errorResponse{
		Code:    err.Code(),
		Message: err.Message(),
		Details: err.DetailsAny(),
	}); encodeErr != nil {
		capitan.Warn(ctx, ResponseWriteError,
			HandlerNameKey.Field(handlerName),
			ErrorKey.Field(encodeErr.Error()),
		)
	}
}

// writeValidationErrorResponse writes detailed validation errors using the standard error format.
func writeValidationErrorResponse(ctx context.Context, w http.ResponseWriter, err error, contentType, handlerName string) {
	// Try to extract check.Result field errors first.
	result := &check.Result{}
	if errors.As(err, &result) {
		fieldErrors := check.GetFieldErrors(result)
		if len(fieldErrors) > 0 {
			fields := make([]ValidationFieldError, len(fieldErrors))
			for i, fe := range fieldErrors {
				fields[i] = ValidationFieldError{
					Field:   fe.Field,
					Message: fe.Message,
				}
			}
			writeError(ctx, w, ErrValidationFailed.WithDetails(ValidationDetails{Fields: fields}), contentType, handlerName)
			return
		}
	}

	// Try legacy ValidationDetails for backward compatibility.
	var details ValidationDetails
	if errors.As(err, &details) {
		writeError(ctx, w, ErrValidationFailed.WithDetails(details), contentType, handlerName)
		return
	}

	// Fallback to generic validation error with message.
	writeError(ctx, w, ErrValidationFailed.WithMessage(err.Error()), contentType, handlerName)
}
