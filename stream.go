package rocco

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zoobz-io/capitan"
	"github.com/zoobz-io/sentinel"
)

// ContentTypeEventStream is the media type for Server-Sent Events responses.
const ContentTypeEventStream = "text/event-stream"

// errClientDisconnected is returned by stream sends after the client has
// disconnected. Handlers can match it with errors.Is.
var errClientDisconnected = errors.New("client disconnected")

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
		return errClientDisconnected
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
		return errClientDisconnected
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
	InputMeta  sentinel.Metadata
	OutputMeta sentinel.Metadata

	// Error definitions with schemas for OpenAPI generation.
	errorDefs map[string]ErrorDefinition

	// Validation flag (checked once at creation time).
	inputValidatable bool // True if In implements Validatable.

	// Runtime configuration.
	responseHeaders map[string]string // Default response headers.

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

	// Parse request body (for POST/PUT streams with initial payload). Streams
	// are SSE and therefore JSON on the wire, so the decoder is fixed.
	input, status, err := decodeRequestBody[In](ctx, r, w, h.spec.Request, json.Unmarshal, h.inputValidatable, h.spec.Name)
	if err != nil {
		return status, err
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

	// Clear the write deadline for this connection. The server's WriteTimeout
	// caps the duration of a single response, which would tear down a long-lived
	// stream mid-flight. Streams are unbounded by design, so the write deadline
	// does not apply. Best-effort: some ResponseWriters do not support deadlines.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		capitan.Warn(ctx, StreamError,
			HandlerNameKey.Field(h.spec.Name),
			ErrorKey.Field(fmt.Sprintf("could not clear write deadline: %v", err)),
		)
	}

	// Write custom response headers first (e.g., CORS). The SSE protocol
	// headers below overwrite them on conflict — they are not overridable.
	for key, value := range h.responseHeaders {
		w.Header().Set(key, value)
	}

	// Set SSE headers. The content type comes from the response contract —
	// the same declaration OpenAPI generation reads.
	w.Header().Set("Content-Type", primaryMediaType(h.spec.Response.MediaTypes))
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
		if errors.Is(err, context.Canceled) || errors.Is(err, errClientDisconnected) {
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
	defs := make([]ErrorDefinition, 0, len(h.errorDefs))
	for _, def := range h.errorDefs {
		defs = append(defs, def)
	}
	return defs
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

	// Check if input type implements Validatable.
	var zeroIn In
	_, inputValidatable := any(zeroIn).(Validatable)

	// Derive the request contract from the input type parameter. Stream
	// request bodies are always JSON — SSE is a text protocol and streams
	// do not participate in codec selection.
	request := RequestContract{
		Kind:       BodyEncoded,
		MaxBytes:   10 * 1024 * 1024, // Default to 10MB.
		MediaTypes: []string{ContentTypeJSON},
	}
	if inputMeta.TypeName == noBodyTypeName {
		request.Kind = BodyNone
		request.MediaTypes = nil
	}

	return &StreamHandler[In, Out]{
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
			Request:        request,
			Response: ResponseContract{
				Kind:       BodyStream,
				Status:     http.StatusOK,
				MediaTypes: []string{ContentTypeEventStream},
			},
			RequiresAuth: false,
			ScopeGroups:  [][]string{},
			RoleGroups:   [][]string{},
			UsageLimits:  []UsageLimit{},
			Tags:         []string{},
		},
		errorDefs:        make(map[string]ErrorDefinition),
		InputMeta:        inputMeta,
		OutputMeta:       outputMeta,
		inputValidatable: inputValidatable,
		responseHeaders:  make(map[string]string),
		middleware:       make([]func(http.Handler) http.Handler, 0),
	}
}

// WithMaxBodySize sets the maximum initial request body size in bytes for this
// stream handler. Applies to the payload of POST/PUT streams. A value of 0
// disables the limit.
func (h *StreamHandler[In, Out]) WithMaxBodySize(size int64) *StreamHandler[In, Out] {
	h.spec.Request.MaxBytes = size
	return h
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
	for _, err := range errs {
		h.errorDefs[err.Code()] = err
	}
	return h
}

// WithMiddleware adds middleware to this handler.
func (h *StreamHandler[In, Out]) WithMiddleware(middleware ...func(http.Handler) http.Handler) *StreamHandler[In, Out] {
	h.middleware = append(h.middleware, middleware...)
	return h
}

// WithResponseHeaders sets default response headers for this stream handler
// (e.g., CORS). The SSE protocol headers (Content-Type, Cache-Control,
// Connection, X-Accel-Buffering) are not overridable and win on conflict.
func (h *StreamHandler[In, Out]) WithResponseHeaders(headers map[string]string) *StreamHandler[In, Out] {
	h.responseHeaders = headers
	return h
}

// WithUsageLimit adds a usage limit check based on identity stats.
// The handler will return 429 Too Many Requests if identity.Stats()[key] >= thresholdFunc(identity).
// The thresholdFunc is called with the identity to allow dynamic limits per user/tenant.
// Usage limits require authentication.
func (h *StreamHandler[In, Out]) WithUsageLimit(key string, thresholdFunc func(Identity) int) *StreamHandler[In, Out] {
	h.spec.UsageLimits = append(h.spec.UsageLimits, UsageLimit{
		Key:           key,
		ThresholdFunc: thresholdFunc,
	})
	// Usage limits require authentication
	h.spec.RequiresAuth = true
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
