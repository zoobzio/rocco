package rocco

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/zoobzio/capitan"
	"github.com/zoobzio/sentinel"
)

// Stream provides a typed interface for sending SSE events.
type Stream[T any] interface {
	// Send sends a data-only event.
	Send(data T) error
	// SendEvent sends a named event with data.
	SendEvent(event string, data T) error
	// SendComment sends a comment (useful for keep-alive).
	SendComment(comment string) error
	// Done returns a channel closed when client disconnects.
	Done() <-chan struct{}
}

// sseStream implements Stream[T] for Server-Sent Events.
type sseStream[T any] struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    <-chan struct{}
	mu      sync.Mutex
	closed  bool
}

// Send sends a data-only event.
func (s *sseStream[T]) Send(data T) error {
	return s.SendEvent("", data)
}

// SendEvent sends a named event with data.
func (s *sseStream[T]) SendEvent(event string, data T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("stream closed")
	}

	// Check if client disconnected
	select {
	case <-s.done:
		s.closed = true
		return errors.New("client disconnected")
	default:
	}

	// Marshal data
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Write event name if provided
	if event != "" {
		if _, err := fmt.Fprintf(s.w, "event: %s\n", event); err != nil {
			s.closed = true
			return fmt.Errorf("failed to write event name: %w", err)
		}
	}

	// Write data
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", jsonData); err != nil {
		s.closed = true
		return fmt.Errorf("failed to write event data: %w", err)
	}

	s.flusher.Flush()
	return nil
}

// SendComment sends a comment (useful for keep-alive).
func (s *sseStream[T]) SendComment(comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("stream closed")
	}

	select {
	case <-s.done:
		s.closed = true
		return errors.New("client disconnected")
	default:
	}

	if _, err := fmt.Fprintf(s.w, ": %s\n\n", comment); err != nil {
		s.closed = true
		return fmt.Errorf("failed to write comment: %w", err)
	}

	s.flusher.Flush()
	return nil
}

// Done returns a channel closed when client disconnects.
func (s *sseStream[T]) Done() <-chan struct{} {
	return s.done
}

// StreamHandler wraps a typed streaming handler function with metadata.
// It implements Endpoint interface for SSE (Server-Sent Events) responses.
type StreamHandler[In, Out any] struct {
	// Core handler function receives Request and Stream for sending events.
	fn func(*Request[In], Stream[Out]) error

	// Declarative specification
	spec HandlerSpec

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
func (h *StreamHandler[In, Out]) Process(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error) {
	// Emit stream handler executing event
	capitan.Debug(ctx, StreamExecuting,
		HandlerNameKey.Field(h.spec.Name),
	)

	// Verify streaming support
	flusher, ok := w.(http.Flusher)
	if !ok {
		capitan.Error(ctx, StreamError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field("streaming not supported"),
		)
		writeError(ctx, w, ErrInternalServer.WithMessage("streaming not supported"), h.spec.Name)
		return http.StatusInternalServerError, errors.New("streaming not supported")
	}

	// Extract and validate parameters.
	params, err := extractParams(ctx, r, h.spec.PathParams, h.spec.QueryParams)
	if err != nil {
		capitan.Error(ctx, RequestParamsInvalid,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		writeError(ctx, w, ErrUnprocessableEntity.WithMessage("invalid parameters").WithCause(err), h.spec.Name)
		return http.StatusUnprocessableEntity, err
	}

	// Parse request body (for POST/PUT streams with initial payload).
	var input In
	if h.InputMeta.TypeName != noBodyTypeName && r.Body != nil {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			capitan.Error(ctx, RequestBodyReadError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(readErr.Error()),
			)
			writeError(ctx, w, ErrBadRequest.WithMessage("failed to read request body").WithCause(readErr), h.spec.Name)
			return http.StatusBadRequest, readErr
		}
		if err := r.Body.Close(); err != nil {
			capitan.Warn(ctx, RequestBodyCloseError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(err.Error()),
			)
		}

		if len(body) > 0 {
			if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
				capitan.Error(ctx, RequestBodyParseError,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(unmarshalErr.Error()),
				)
				writeError(ctx, w, ErrUnprocessableEntity.WithMessage("invalid request body").WithCause(unmarshalErr), h.spec.Name)
				return http.StatusUnprocessableEntity, unmarshalErr
			}

			// Validate input.
			if inputErr := h.validator.Struct(input); inputErr != nil {
				capitan.Warn(ctx, RequestValidationInputFailed,
					HandlerNameKey.Field(h.spec.Name),
					ErrorKey.Field(inputErr.Error()),
				)
				writeValidationErrorResponse(ctx, w, inputErr, h.spec.Name)
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

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	w.WriteHeader(http.StatusOK)

	// Emit stream started event
	capitan.Info(ctx, StreamStarted,
		HandlerNameKey.Field(h.spec.Name),
	)

	// Create stream
	stream := &sseStream[Out]{
		w:       w,
		flusher: flusher,
		done:    ctx.Done(),
	}

	// Call user handler (blocks until stream ends)
	if err := h.fn(req, stream); err != nil {
		// Check if this is a rocco Error.
		if e := getRoccoError(err); e != nil {
			capitan.Warn(ctx, StreamError,
				HandlerNameKey.Field(h.spec.Name),
				ErrorKey.Field(err.Error()),
			)
			// Cannot write error response after headers sent, just log
			return http.StatusOK, err
		}

		// Check for client disconnect
		if errors.Is(err, context.Canceled) || err.Error() == "client disconnected" {
			capitan.Info(ctx, StreamClientDisconnected,
				HandlerNameKey.Field(h.spec.Name),
			)
			return http.StatusOK, nil
		}

		// Unexpected error
		capitan.Error(ctx, StreamError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(err.Error()),
		)
		return http.StatusOK, err
	}

	// Emit stream ended event
	capitan.Info(ctx, StreamEnded,
		HandlerNameKey.Field(h.spec.Name),
	)

	return http.StatusOK, nil
}

// Spec implements Endpoint.
func (h *StreamHandler[In, Out]) Spec() HandlerSpec {
	return h.spec
}

// ErrorDefs implements Endpoint.
func (h *StreamHandler[In, Out]) ErrorDefs() []ErrorDefinition {
	return h.errorDefs
}

// Middleware implements Endpoint.
func (h *StreamHandler[In, Out]) Middleware() []func(http.Handler) http.Handler {
	return h.middleware
}

// Close implements Endpoint.
func (*StreamHandler[In, Out]) Close() error {
	return nil
}

// NewStreamHandler creates a new typed streaming handler with sentinel metadata.
func NewStreamHandler[In, Out any](name string, method, path string, fn func(*Request[In], Stream[Out]) error) *StreamHandler[In, Out] {
	inputMeta := sentinel.Scan[In]()
	outputMeta := sentinel.Scan[Out]()

	return &StreamHandler[In, Out]{
		fn: fn,
		spec: HandlerSpec{
			Name:           name,
			Method:         method,
			Path:           path,
			PathParams:     []string{},
			QueryParams:    []string{},
			InputTypeName:  inputMeta.TypeName,
			OutputTypeName: outputMeta.TypeName,
			SuccessStatus:  http.StatusOK,
			ErrorCodes:     []int{},
			RequiresAuth:   false,
			ScopeGroups:    [][]string{},
			RoleGroups:     [][]string{},
			UsageLimits:    []UsageLimit{},
			Tags:           []string{},
			IsStream:       true,
		},
		InputMeta:  inputMeta,
		OutputMeta: outputMeta,
		validator:  validator.New(),
		middleware: make([]func(http.Handler) http.Handler, 0),
	}
}

// WithSummary sets the OpenAPI summary.
func (h *StreamHandler[In, Out]) WithSummary(summary string) *StreamHandler[In, Out] {
	h.spec.Summary = summary
	return h
}

// WithDescription sets the OpenAPI description.
func (h *StreamHandler[In, Out]) WithDescription(desc string) *StreamHandler[In, Out] {
	h.spec.Description = desc
	return h
}

// WithTags sets the OpenAPI tags.
func (h *StreamHandler[In, Out]) WithTags(tags ...string) *StreamHandler[In, Out] {
	h.spec.Tags = tags
	return h
}

// WithPathParams specifies required path parameters.
func (h *StreamHandler[In, Out]) WithPathParams(params ...string) *StreamHandler[In, Out] {
	h.spec.PathParams = params
	return h
}

// WithQueryParams specifies required query parameters.
func (h *StreamHandler[In, Out]) WithQueryParams(params ...string) *StreamHandler[In, Out] {
	h.spec.QueryParams = params
	return h
}

// WithErrors declares which errors this handler may return.
// Note: Errors can only be returned before the stream starts.
func (h *StreamHandler[In, Out]) WithErrors(errs ...ErrorDefinition) *StreamHandler[In, Out] {
	h.errorDefs = append(h.errorDefs, errs...)
	for _, err := range errs {
		h.spec.ErrorCodes = append(h.spec.ErrorCodes, err.Status())
	}
	return h
}

// WithMiddleware adds middleware to this handler.
func (h *StreamHandler[In, Out]) WithMiddleware(middleware ...func(http.Handler) http.Handler) *StreamHandler[In, Out] {
	h.middleware = append(h.middleware, middleware...)
	return h
}

// WithAuthentication marks this handler as requiring authentication.
func (h *StreamHandler[In, Out]) WithAuthentication() *StreamHandler[In, Out] {
	h.spec.RequiresAuth = true
	return h
}

// WithScopes adds a scope requirement group.
func (h *StreamHandler[In, Out]) WithScopes(scopes ...string) *StreamHandler[In, Out] {
	if len(scopes) > 0 {
		h.spec.ScopeGroups = append(h.spec.ScopeGroups, scopes)
		h.spec.RequiresAuth = true
	}
	return h
}

// WithRoles adds a role requirement group.
func (h *StreamHandler[In, Out]) WithRoles(roles ...string) *StreamHandler[In, Out] {
	if len(roles) > 0 {
		h.spec.RoleGroups = append(h.spec.RoleGroups, roles)
		h.spec.RequiresAuth = true
	}
	return h
}
