package rocco

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/zoobzio/capitan"
)

type Engine struct {
	config              *EngineConfig
	server              *http.Server
	chiRouter           chi.Router
	middleware          []func(http.Handler) http.Handler
	handlers            []RouteHandler // Registered handlers for OpenAPI generation
	ctx                 context.Context
	cancel              context.CancelFunc
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

	// Emit engine created event
	capitan.Emit(ctx, EventEngineCreated,
		KeyHost.Field(config.Host),
		KeyPort.Field(config.Port),
	)

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

		// Emit handler registered event
		capitan.Emit(e.ctx, EventHandlerRegistered,
			KeyHandlerName.Field(handler.Name()),
			KeyMethod.Field(handler.Method()),
			KeyPath.Field(handler.Path()),
		)
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
func (*Engine) adaptHandler(handler RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Emit request received event
		capitan.Emit(ctx, EventRequestReceived,
			KeyMethod.Field(r.Method),
			KeyPath.Field(r.URL.Path),
			KeyHandlerName.Field(handler.Name()),
		)

		// Handler processes and writes response
		err := handler.Process(ctx, r, w)

		// Emit request completion event
		if err != nil {
			capitan.Emit(ctx, EventRequestFailed,
				KeyMethod.Field(r.Method),
				KeyPath.Field(r.URL.Path),
				KeyHandlerName.Field(handler.Name()),
				KeyError.Field(err.Error()),
			)
		} else {
			capitan.Emit(ctx, EventRequestCompleted,
				KeyMethod.Field(r.Method),
				KeyPath.Field(r.URL.Path),
				KeyHandlerName.Field(handler.Name()),
			)
		}
	}
}

// Start begins listening for HTTP requests.
// This method blocks until the server is shutdown.
func (e *Engine) Start() error {
	// Emit engine starting event
	capitan.Emit(e.ctx, EventEngineStarting,
		KeyHost.Field(e.config.Host),
		KeyPort.Field(e.config.Port),
		KeyAddress.Field(e.server.Addr),
	)

	err := e.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown performs a graceful shutdown of the engine.
func (e *Engine) Shutdown(ctx context.Context) error {
	// Emit shutdown started event
	capitan.Emit(ctx, EventEngineShutdownStarted)

	// Shutdown HTTP server (waits for active connections to finish)
	err := e.server.Shutdown(ctx)

	// Cancel engine context
	e.cancel()

	// Emit shutdown complete event
	if err != nil {
		capitan.Emit(context.Background(), EventEngineShutdownComplete,
			KeyGraceful.Field(false),
			KeyError.Field(err.Error()),
		)
	} else {
		capitan.Emit(context.Background(), EventEngineShutdownComplete,
			KeyGraceful.Field(true),
		)
	}

	// Shutdown event system
	capitan.Shutdown()

	return err
}
