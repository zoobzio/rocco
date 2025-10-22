package rocco

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zoobzio/hookz"
	"github.com/zoobzio/metricz"
	"github.com/zoobzio/tracez"
)

// Trace spans for Engine.
const (
	TraceRequestReceived tracez.Key = "engine.request.received"
	TraceRequestProcess  tracez.Key = "engine.request.process"
	TraceShutdown        tracez.Key = "engine.shutdown"
)

// Trace tags.
const (
	TagMethod     tracez.Tag = "method"
	TagPath       tracez.Tag = "path"
	TagRejected   tracez.Tag = "rejected"
	TagDurationMs tracez.Tag = "duration_ms"
)

// Metric keys for Engine.
const (
	// Request metrics.
	MetricRequestsReceived  metricz.Key = "engine.requests.received"
	MetricRequestsRejected  metricz.Key = "engine.requests.rejected"
	MetricRequestsCompleted metricz.Key = "engine.requests.completed"
	MetricRequestDuration   metricz.Key = "engine.request.duration_ms"
)

// Hook events for Engine.
const (
	// Request lifecycle hooks.
	HookRequestReceived  hookz.Key = "engine.request.received"  // Request arrived at engine.
	HookRequestCompleted hookz.Key = "engine.request.completed" // Request finished processing.
	HookRequestRejected  hookz.Key = "engine.request.rejected"  // Request rejected (error).

	// Shutdown hooks.
	HookShutdownStarted  hookz.Key = "engine.shutdown.started"  // Shutdown initiated.
	HookShutdownComplete hookz.Key = "engine.shutdown.complete" // Shutdown finished.
)

type Engine struct {
	config              *EngineConfig
	server              *http.Server
	chiRouter           chi.Router
	middleware          []func(http.Handler) http.Handler
	handlers            []RouteHandler // Registered handlers for OpenAPI generation
	ctx                 context.Context
	cancel              context.CancelFunc
	metrics             *metricz.Registry
	tracer              *tracez.Tracer
	hooks               *hookz.Hooks[*Event]
	defaultHandlersOnce sync.Once
}

// NewEngine creates a new Engine with the given configuration.
// If config is nil, uses DefaultConfig.
func NewEngine(config *EngineConfig) *Engine {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create Chi router
	r := chi.NewRouter()

	e := &Engine{
		config:     config,
		chiRouter:  r,
		middleware: make([]func(http.Handler) http.Handler, 0),
		ctx:        ctx,
		cancel:     cancel,
		metrics:    metricz.New(),
		tracer:     tracez.New(),
		hooks:      hookz.New[*Event](),
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	e.server = &http.Server{
		Addr:         addr,
		Handler:      e.chiRouter,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	return e
}

// WithMiddleware adds global middleware to the engine and returns the engine for chaining.
func (e *Engine) WithMiddleware(middleware ...func(http.Handler) http.Handler) *Engine {
	for _, mw := range middleware {
		e.chiRouter.Use(mw)
	}
	return e
}

// WithHandlers adds one or more RouteHandlers to the engine and returns the engine for chaining.
func (e *Engine) WithHandlers(handlers ...RouteHandler) *Engine {
	// Ensure default handlers are registered first (only once)
	e.ensureDefaultHandlers()

	for _, handler := range handlers {
		// Store handler for OpenAPI generation.
		e.handlers = append(e.handlers, handler)

		// Adapt our handler to http.HandlerFunc.
		httpHandler := e.adaptHandler(handler)

		// Apply handler-specific middleware if available.
		middleware := handler.Middleware()
		if len(middleware) > 0 {
			e.chiRouter.With(middleware...).Method(handler.Method(), handler.Path(), httpHandler)
		} else {
			// Register with Chi (no handler middleware).
			e.chiRouter.Method(handler.Method(), handler.Path(), httpHandler)
		}
	}
	return e
}

// ensureDefaultHandlers sets up OpenAPI spec and docs handlers at /openapi and /docs (once).
func (e *Engine) ensureDefaultHandlers() {
	e.defaultHandlersOnce.Do(func() {
		e.registerDefaultHandlers()
	})
}

// registerDefaultHandlers sets up OpenAPI spec and docs handlers at /openapi and /docs.
func (e *Engine) registerDefaultHandlers() {
	// OpenAPI spec handler at /openapi
	e.chiRouter.Get("/openapi", func(w http.ResponseWriter, _ *http.Request) {
		spec := e.GenerateOpenAPI(Info{
			Title:   "API",
			Version: "1.0.0",
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Marshal to pretty-printed JSON.
		data, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			http.Error(w, "failed to generate OpenAPI spec", http.StatusInternalServerError)
			return
		}

		w.Write(data)
	})

	// Docs handler at /docs
	e.chiRouter.Get("/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
    <script id="api-reference" data-url="/openapi"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

		w.Write([]byte(html))
	})
}

// adaptHandler converts a RouteHandler to http.HandlerFunc.
func (e *Engine) adaptHandler(handler RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Start trace for this request
		ctx, span := e.tracer.StartSpan(r.Context(), TraceRequestReceived)
		defer span.Finish()
		span.SetTag(TagMethod, r.Method)
		span.SetTag(TagPath, r.URL.Path)

		// Emit request received event
		if e.hooks.ListenerCount(HookRequestReceived) > 0 {
			event := NewRequestEvent("request.received", r, nil)
			e.hooks.Emit(ctx, HookRequestReceived, event)
		}

		// Track incoming request
		e.metrics.Counter(MetricRequestsReceived).Inc()
		start := time.Now()

		// Handler processes and writes response
		err := handler.Process(ctx, r, w)

		duration := time.Since(start)
		e.metrics.Timer(MetricRequestDuration).Record(duration)

		if err != nil {
			// Error occurred
			e.metrics.Counter(MetricRequestsRejected).Inc()
			span.SetTag(TagRejected, "handler_error")

			// Emit rejection event
			if e.hooks.ListenerCount(HookRequestRejected) > 0 {
				event := NewRequestEvent("request.rejected", r, map[string]any{
					"error":       err.Error(),
					"duration_ms": float64(duration) / float64(time.Millisecond),
				})
				e.hooks.Emit(ctx, HookRequestRejected, event)
			}
			return
		}

		// Success
		e.metrics.Counter(MetricRequestsCompleted).Inc()

		// Emit completion event
		if e.hooks.ListenerCount(HookRequestCompleted) > 0 {
			event := NewRequestEvent("request.completed", r, map[string]any{
				"duration_ms": float64(duration) / float64(time.Millisecond),
			})
			e.hooks.Emit(ctx, HookRequestCompleted, event)
		}
	}
}

// Metrics returns the engine's metrics registry.
func (e *Engine) Metrics() *metricz.Registry {
	return e.metrics
}

// Tracer returns the engine's tracer.
func (e *Engine) Tracer() *tracez.Tracer {
	return e.tracer
}

// Hooks returns the engine's hooks registry.
func (e *Engine) Hooks() *hookz.Hooks[*Event] {
	return e.hooks
}

// OnRequestReceived registers a handler for when a request arrives at the engine.
func (e *Engine) OnRequestReceived(handler func(context.Context, *Event) error) error {
	_, err := e.hooks.Hook(HookRequestReceived, handler)
	return err
}

// OnRequestCompleted registers a handler for when a request finishes processing.
func (e *Engine) OnRequestCompleted(handler func(context.Context, *Event) error) error {
	_, err := e.hooks.Hook(HookRequestCompleted, handler)
	return err
}

// OnRequestRejected registers a handler for when a request is rejected due to error.
func (e *Engine) OnRequestRejected(handler func(context.Context, *Event) error) error {
	_, err := e.hooks.Hook(HookRequestRejected, handler)
	return err
}

// OnShutdownStarted registers a handler for when shutdown is initiated.
func (e *Engine) OnShutdownStarted(handler func(context.Context, *Event) error) error {
	_, err := e.hooks.Hook(HookShutdownStarted, handler)
	return err
}

// OnShutdownComplete registers a handler for when shutdown finishes.
func (e *Engine) OnShutdownComplete(handler func(context.Context, *Event) error) error {
	_, err := e.hooks.Hook(HookShutdownComplete, handler)
	return err
}

// Start begins listening for HTTP requests.
// This method blocks until the server is shutdown.
func (e *Engine) Start() error {
	slog.Info("starting server", "host", e.config.Host, "port", e.config.Port)
	err := e.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown performs a graceful shutdown of the engine.
func (e *Engine) Shutdown(ctx context.Context) error {
	// Start shutdown span
	_, span := e.tracer.StartSpan(ctx, TraceShutdown)
	defer span.Finish()

	slog.Info("starting graceful shutdown")

	// Emit shutdown started event
	shutdownEvent := NewSystemEvent("shutdown.started", nil)
	e.hooks.Emit(ctx, HookShutdownStarted, shutdownEvent)

	// Shutdown HTTP server (waits for active connections to finish)
	err := e.server.Shutdown(ctx)

	// Cancel engine context
	e.cancel()

	if err != nil {
		slog.Error("shutdown error", "error", err)
		// Emit shutdown complete event (with error)
		errorEvent := NewSystemEvent("shutdown.complete", map[string]any{
			"graceful": false,
			"error":    err.Error(),
		})
		e.hooks.Emit(context.Background(), HookShutdownComplete, errorEvent)
		return err
	}

	slog.Info("graceful shutdown complete")

	// Emit shutdown complete event
	completeEvent := NewSystemEvent("shutdown.complete", map[string]any{
		"graceful": true,
	})
	e.hooks.Emit(context.Background(), HookShutdownComplete, completeEvent)

	return nil
}
