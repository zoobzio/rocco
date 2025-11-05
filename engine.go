package rocco

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zoobzio/capitan"
	"github.com/zoobzio/openapi"
)

type Engine struct {
	config              *EngineConfig
	server              *http.Server
	chiRouter           chi.Router
	middleware          []func(http.Handler) http.Handler
	handlers            []Endpoint // Registered handlers for OpenAPI generation
	extractIdentity     func(context.Context, *http.Request) (Identity, error)
	ctx                 context.Context
	cancel              context.CancelFunc
	defaultHandlersOnce sync.Once
	spec                *EngineSpec // OpenAPI specification configuration
}

// NewEngine creates a new Engine with identity extraction.
// The extractIdentity function is called for handlers that require authentication.
// Pass nil for extractIdentity if you don't need authentication.
func NewEngine(
	host string,
	port int,
	extractIdentity func(context.Context, *http.Request) (Identity, error),
) *Engine {
	config := &EngineConfig{
		Host:         host,
		Port:         port,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create Chi router
	r := chi.NewRouter()

	e := &Engine{
		config:          config,
		chiRouter:       r,
		middleware:      make([]func(http.Handler) http.Handler, 0),
		extractIdentity: extractIdentity,
		ctx:             ctx,
		cancel:          cancel,
		spec:            DefaultEngineSpec(),
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
	capitan.Debug(ctx, EngineCreated,
		HostKey.Field(config.Host),
		PortKey.Field(config.Port),
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

// WithSpec sets the engine specification for OpenAPI generation.
func (e *Engine) WithSpec(spec *EngineSpec) *Engine {
	e.spec = spec
	return e
}

// WithOpenAPIInfo sets the OpenAPI Info metadata.
func (e *Engine) WithOpenAPIInfo(info openapi.Info) *Engine {
	e.spec.Info = info
	return e
}

// WithTag adds a tag with description to the OpenAPI specification.
// Tags are used to group operations in the documentation.
func (e *Engine) WithTag(name, description string) *Engine {
	// Check if tag already exists and update it
	for i, tag := range e.spec.Tags {
		if tag.Name == name {
			e.spec.Tags[i].Description = description
			return e
		}
	}
	// Add new tag
	e.spec.Tags = append(e.spec.Tags, openapi.Tag{
		Name:        name,
		Description: description,
	})
	return e
}

// Router returns the underlying chi.Router for advanced use cases.
// This allows power users to register custom routes that won't appear in OpenAPI documentation.
func (e *Engine) Router() chi.Router {
	return e.chiRouter
}

// WithHandlers adds one or more Endpoints to the engine and returns the engine for chaining.
func (e *Engine) WithHandlers(handlers ...Endpoint) *Engine {
	// Ensure default handlers are registered first (only once)
	e.ensureDefaultHandlers()

	for _, handler := range handlers {
		// Store handler for OpenAPI generation.
		e.handlers = append(e.handlers, handler)

		// Adapt our handler to http.HandlerFunc.
		httpHandler := e.adaptHandler(handler)

		// Build middleware stack: handler middleware + auth middleware (if handler requires it)
		handlerSpec := handler.Spec()
		middleware := handler.Middleware()

		// Add authentication middleware if handler requires it
		if handlerSpec.RequiresAuth && e.extractIdentity != nil {
			authMiddleware := e.buildAuthMiddleware()
			middleware = append(middleware, authMiddleware)

			// Add authorization middleware if handler has scope/role requirements
			if len(handlerSpec.ScopeGroups) > 0 || len(handlerSpec.RoleGroups) > 0 {
				authzMiddleware := e.buildAuthorizationMiddleware(handler)
				middleware = append(middleware, authzMiddleware)
			}

			// Add usage limit middleware if handler has usage limits
			if len(handlerSpec.UsageLimits) > 0 {
				usageLimitMiddleware := e.buildUsageLimitMiddleware(handler)
				middleware = append(middleware, usageLimitMiddleware)
			}
		}

		if len(middleware) > 0 {
			e.chiRouter.With(middleware...).Method(handlerSpec.Method, handlerSpec.Path, httpHandler)
		} else {
			// Register with Chi (no handler middleware).
			e.chiRouter.Method(handlerSpec.Method, handlerSpec.Path, httpHandler)
		}

		// Emit handler registered event
		capitan.Debug(e.ctx, HandlerRegistered,
			HandlerNameKey.Field(handlerSpec.Name),
			MethodKey.Field(handlerSpec.Method),
			PathKey.Field(handlerSpec.Path),
		)
	}
	return e
}

// buildAuthMiddleware creates authentication middleware using the extractIdentity callback.
func (e *Engine) buildAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract identity
			identity, err := e.extractIdentity(ctx, r)
			if err != nil {
				capitan.Warn(ctx, AuthenticationFailed,
					MethodKey.Field(r.Method),
					PathKey.Field(r.URL.Path),
					ErrorKey.Field(err.Error()),
				)
				writeErrorResponse(w, http.StatusUnauthorized)
				return
			}

			// Store identity in context
			ctx = context.WithValue(ctx, identityContextKey, identity)

			capitan.Debug(ctx, AuthenticationSucceeded,
				MethodKey.Field(r.Method),
				PathKey.Field(r.URL.Path),
				IdentityIDKey.Field(identity.ID()),
				TenantIDKey.Field(identity.TenantID()),
			)

			// Continue with enriched context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// identityContextKey is the context key for storing Identity.
type contextKey string

const identityContextKey contextKey = "rocco_identity"

// buildAuthorizationMiddleware creates middleware that checks scope and role requirements.
// Scope/role groups use OR within each group, AND across groups.
func (*Engine) buildAuthorizationMiddleware(handler Endpoint) func(http.Handler) http.Handler {
	handlerSpec := handler.Spec()
	scopeGroups := handlerSpec.ScopeGroups
	roleGroups := handlerSpec.RoleGroups

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract identity from context (should exist from auth middleware)
			val := ctx.Value(identityContextKey)
			if val == nil {
				writeErrorResponse(w, http.StatusForbidden)
				return
			}

			identity, ok := val.(Identity)
			if !ok {
				writeErrorResponse(w, http.StatusForbidden)
				return
			}

			// Check scope requirements (AND across groups, OR within group)
			for _, scopeGroup := range scopeGroups {
				hasAnyScope := false
				for _, scope := range scopeGroup {
					if identity.HasScope(scope) {
						hasAnyScope = true
						break
					}
				}
				if !hasAnyScope {
					// Missing required scope group
					capitan.Warn(ctx, AuthorizationScopeDenied,
						MethodKey.Field(r.Method),
						PathKey.Field(r.URL.Path),
						IdentityIDKey.Field(identity.ID()),
						RequiredScopesKey.Field(strings.Join(scopeGroup, ",")),
					)
					writeErrorResponse(w, http.StatusForbidden)
					return
				}
			}

			// Check role requirements (AND across groups, OR within group)
			for _, roleGroup := range roleGroups {
				hasAnyRole := false
				for _, role := range roleGroup {
					if identity.HasRole(role) {
						hasAnyRole = true
						break
					}
				}
				if !hasAnyRole {
					// Missing required role group
					capitan.Warn(ctx, AuthorizationRoleDenied,
						MethodKey.Field(r.Method),
						PathKey.Field(r.URL.Path),
						IdentityIDKey.Field(identity.ID()),
						RequiredRolesKey.Field(strings.Join(roleGroup, ",")),
					)
					writeErrorResponse(w, http.StatusForbidden)
					return
				}
			}

			// All checks passed
			capitan.Debug(ctx, AuthorizationSucceeded,
				MethodKey.Field(r.Method),
				PathKey.Field(r.URL.Path),
				IdentityIDKey.Field(identity.ID()),
			)
			next.ServeHTTP(w, r)
		})
	}
}

// buildUsageLimitMiddleware creates middleware that checks usage limits from identity stats.
func (*Engine) buildUsageLimitMiddleware(handler Endpoint) func(http.Handler) http.Handler {
	handlerSpec := handler.Spec()
	usageLimits := handlerSpec.UsageLimits

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract identity from context (should exist from auth middleware)
			val := ctx.Value(identityContextKey)
			if val == nil {
				writeErrorResponse(w, http.StatusForbidden)
				return
			}

			identity, ok := val.(Identity)
			if !ok {
				writeErrorResponse(w, http.StatusForbidden)
				return
			}

			// Get identity stats
			stats := identity.Stats()
			if stats == nil {
				stats = make(map[string]int)
			}

			// Check each usage limit
			for _, limit := range usageLimits {
				threshold := limit.ThresholdFunc(identity)
				currentValue := stats[limit.Key]
				if currentValue >= threshold {
					// Usage limit exceeded
					capitan.Warn(ctx, RateLimitExceeded,
						MethodKey.Field(r.Method),
						PathKey.Field(r.URL.Path),
						IdentityIDKey.Field(identity.ID()),
						LimitKeyKey.Field(limit.Key),
						CurrentValueKey.Field(currentValue),
						ThresholdKey.Field(threshold),
					)
					writeErrorResponse(w, http.StatusTooManyRequests)
					return
				}
			}

			// All checks passed
			next.ServeHTTP(w, r)
		})
	}
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
		spec := e.GenerateOpenAPI(nil)

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

// adaptHandler converts a Endpoint to http.HandlerFunc.
func (*Engine) adaptHandler(handler Endpoint) http.HandlerFunc {
	handlerSpec := handler.Spec()

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		startTime := time.Now()

		// Emit request received event
		capitan.Debug(ctx, RequestReceived,
			MethodKey.Field(r.Method),
			PathKey.Field(r.URL.Path),
			HandlerNameKey.Field(handlerSpec.Name),
		)

		// Handler processes and writes response
		status, err := handler.Process(ctx, r, w)

		// Calculate duration
		durationMs := time.Since(startTime).Milliseconds()

		// Emit request completion event
		if err != nil {
			capitan.Error(ctx, RequestFailed,
				MethodKey.Field(r.Method),
				PathKey.Field(r.URL.Path),
				HandlerNameKey.Field(handlerSpec.Name),
				StatusCodeKey.Field(status),
				DurationMsKey.Field(durationMs),
				ErrorKey.Field(err.Error()),
			)
		} else {
			capitan.Info(ctx, RequestCompleted,
				MethodKey.Field(r.Method),
				PathKey.Field(r.URL.Path),
				HandlerNameKey.Field(handlerSpec.Name),
				StatusCodeKey.Field(status),
				DurationMsKey.Field(durationMs),
			)
		}
	}
}

// Start begins listening for HTTP requests.
// This method blocks until the server is shutdown.
func (e *Engine) Start() error {
	// Emit engine starting event
	capitan.Info(e.ctx, EngineStarting,
		HostKey.Field(e.config.Host),
		PortKey.Field(e.config.Port),
		AddressKey.Field(e.server.Addr),
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
	capitan.Info(ctx, EngineShutdownStarted)

	// Shutdown HTTP server (waits for active connections to finish)
	err := e.server.Shutdown(ctx)

	// Cancel engine context
	e.cancel()

	// Emit shutdown complete event
	if err != nil {
		capitan.Error(context.Background(), EngineShutdownComplete,
			GracefulKey.Field(false),
			ErrorKey.Field(err.Error()),
		)
	} else {
		capitan.Info(context.Background(), EngineShutdownComplete,
			GracefulKey.Field(true),
		)
	}

	// Shutdown event system
	capitan.Shutdown()

	return err
}
