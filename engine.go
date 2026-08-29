package rocco

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zoobz-io/capitan"
	"github.com/zoobz-io/openapi"
)

// chain wraps a handler with middleware (applied in reverse order).
func chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// Engine is the core HTTP server that manages routing, middleware, and handler registration.
type Engine struct {
	config              *EngineConfig
	server              *http.Server
	mux                 *http.ServeMux
	globalMiddleware    []func(http.Handler) http.Handler
	handlers            []Endpoint // Registered handlers for OpenAPI generation
	models              []*Model   // Standalone models for OpenAPI schema generation
	extractIdentity     func(context.Context, *http.Request) (Identity, error)
	ctx                 context.Context
	cancel              context.CancelFunc
	defaultHandlersOnce sync.Once
	spec                *EngineSpec // OpenAPI specification configuration
	cachedOpenAPISpec   []byte      // Cached JSON-encoded OpenAPI spec
	openAPIOnce         sync.Once   // Ensures OpenAPI spec is generated only once
	codec               Codec       // Default codec for handlers (nil = use handler default)
	tlsConfig           *tls.Config // TLS configuration (nil = plain HTTP)
}

// NewEngine creates a new Engine.
// Use WithAuthenticator to configure identity extraction for authenticated handlers.
func NewEngine() *Engine {
	config := &EngineConfig{
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create stdlib ServeMux
	mux := http.NewServeMux()

	e := &Engine{
		config:           config,
		mux:              mux,
		globalMiddleware: make([]func(http.Handler) http.Handler, 0),
		ctx:              ctx,
		cancel:           cancel,
		spec:             DefaultEngineSpec(),
	}

	// Emit engine created event
	capitan.Debug(ctx, EngineCreated)

	return e
}

// WithMiddleware adds global middleware to the engine and returns the engine for chaining.
func (e *Engine) WithMiddleware(middleware ...func(http.Handler) http.Handler) *Engine {
	e.globalMiddleware = append(e.globalMiddleware, middleware...)
	return e
}

// WithAuthenticator sets the identity extraction function for authenticated handlers.
// The extractor is called for handlers that require authentication (via WithAuthentication).
func (e *Engine) WithAuthenticator(extractor func(context.Context, *http.Request) (Identity, error)) *Engine {
	e.extractIdentity = extractor
	return e
}

// WithCodec sets the default codec for all handlers registered with this engine.
// Handlers that explicitly call WithCodec() will use their own codec instead.
func (e *Engine) WithCodec(codec Codec) *Engine {
	e.codec = codec
	return e
}

// WithTLSConfig sets the TLS configuration for the engine's HTTP server.
// When set, the server will use TLS (HTTPS) instead of plain HTTP.
func (e *Engine) WithTLSConfig(config *tls.Config) *Engine {
	e.tlsConfig = config
	return e
}

// WithTimeouts sets the server's read, write, and idle timeouts.
//
// WriteTimeout caps the duration of a single response. Stream handlers are
// exempt: they clear their own write deadline so a long-lived stream is not
// terminated at WriteTimeout. ReadTimeout and IdleTimeout apply to all routes.
//
// A zero value disables the corresponding timeout.
func (e *Engine) WithTimeouts(read, write, idle time.Duration) *Engine {
	e.config.ReadTimeout = read
	e.config.WriteTimeout = write
	e.config.IdleTimeout = idle
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

// WithTagGroup adds a tag group for hierarchical tag organization.
// Tag groups are rendered via the x-tagGroups vendor extension.
func (e *Engine) WithTagGroup(name string, tags ...string) *Engine {
	// Check if group already exists and update it
	for i, group := range e.spec.TagGroups {
		if group.Name == name {
			e.spec.TagGroups[i].Tags = tags
			return e
		}
	}
	// Add new group
	e.spec.TagGroups = append(e.spec.TagGroups, openapi.TagGroup{
		Name: name,
		Tags: tags,
	})
	return e
}

// Router returns the underlying http.ServeMux for advanced use cases.
// This allows power users to register custom routes that won't appear in OpenAPI documentation.
func (e *Engine) Router() *http.ServeMux {
	return e.mux
}

// WithModels registers standalone types into the OpenAPI component schemas.
// These types don't need to be handler input or output types — they are included
// in the spec for use by features like discriminated unions or external references.
func (e *Engine) WithModels(models ...*Model) *Engine {
	e.models = append(e.models, models...)
	return e
}

// WithHandlers adds one or more Endpoints to the engine and returns the engine for chaining.
//
// Threading model: All handler registration must complete before calling Start().
// Calling WithHandlers concurrently or after Start() results in undefined behavior.
// This follows the standard Go pattern for HTTP server configuration.
func (e *Engine) WithHandlers(handlers ...Endpoint) *Engine {
	// Ensure default handlers are registered first (only once)
	e.ensureDefaultHandlers()

	for _, handler := range handlers {
		// Apply engine's default codec if handler supports it and engine has one
		if e.codec != nil {
			if ca, ok := handler.(codecApplier); ok {
				ca.applyDefaultCodec(e.codec)
			}
		}

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

		// Compose all middleware: global + handler-specific
		allMiddleware := make([]func(http.Handler) http.Handler, 0, len(e.globalMiddleware)+len(middleware))
		allMiddleware = append(allMiddleware, e.globalMiddleware...)
		allMiddleware = append(allMiddleware, middleware...)
		wrappedHandler := chain(httpHandler, allMiddleware...)

		// Register with stdlib mux using "METHOD /path" pattern
		pattern := handlerSpec.Method + " " + handlerSpec.Path
		e.mux.Handle(pattern, wrappedHandler)

		// Emit handler registered event
		capitan.Debug(e.ctx, HandlerRegistered,
			HandlerNameKey.Field(handlerSpec.Name),
			MethodKey.Field(handlerSpec.Method),
			PathKey.Field(handlerSpec.Path),
		)

		// A stream handler's wire format is fixed: SSE frames carrying JSON.
		// A non-JSON engine codec does not reach it. Warn so the mismatch is
		// visible at registration instead of silently ignored.
		if e.codec != nil && e.codec.ContentType() != ContentTypeJSON && handlerSpec.Response.Kind == BodyStream {
			capitan.Warn(e.ctx, HandlerCodecMismatch,
				HandlerNameKey.Field(handlerSpec.Name),
				ErrorKey.Field(fmt.Sprintf("engine codec %q does not apply: streams always use JSON in SSE frames", e.codec.ContentType())),
			)
		}
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
				writeError(ctx, w, ErrUnauthorized, "auth")
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
				writeError(ctx, w, ErrForbidden.WithMessage("identity not found"), handlerSpec.Name)
				return
			}

			identity, ok := val.(Identity)
			if !ok {
				writeError(ctx, w, ErrForbidden.WithMessage("invalid identity"), handlerSpec.Name)
				return
			}

			// Check scope/role requirements (AND across groups, OR within group)
			failedScopes, failedRoles, ok := satisfiesRequirements(identity, scopeGroups, roleGroups)
			if !ok {
				if failedScopes != nil {
					capitan.Warn(ctx, AuthorizationScopeDenied,
						MethodKey.Field(r.Method),
						PathKey.Field(r.URL.Path),
						IdentityIDKey.Field(identity.ID()),
						RequiredScopesKey.Field(strings.Join(failedScopes, ",")),
					)
					writeError(ctx, w, ErrForbidden.WithMessage("insufficient scope"), handlerSpec.Name)
					return
				}
				capitan.Warn(ctx, AuthorizationRoleDenied,
					MethodKey.Field(r.Method),
					PathKey.Field(r.URL.Path),
					IdentityIDKey.Field(identity.ID()),
					RequiredRolesKey.Field(strings.Join(failedRoles, ",")),
				)
				writeError(ctx, w, ErrForbidden.WithMessage("insufficient role"), handlerSpec.Name)
				return
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
				writeError(ctx, w, ErrForbidden.WithMessage("identity not found"), handlerSpec.Name)
				return
			}

			identity, ok := val.(Identity)
			if !ok {
				writeError(ctx, w, ErrForbidden.WithMessage("invalid identity"), handlerSpec.Name)
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
					writeError(ctx, w, ErrTooManyRequests, handlerSpec.Name)
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
	e.mux.HandleFunc("GET /openapi", func(w http.ResponseWriter, r *http.Request) {
		// Generate and cache spec on first request (cached forever after)
		e.openAPIOnce.Do(func() {
			spec := e.GenerateOpenAPI(nil)
			data, err := json.MarshalIndent(spec, "", "  ")
			if err != nil {
				// Marshal failure is a programming error - spec remains nil
				return
			}
			e.cachedOpenAPISpec = data
		})

		if e.cachedOpenAPISpec == nil {
			http.Error(w, "failed to generate OpenAPI spec", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(e.cachedOpenAPISpec); err != nil {
			capitan.Warn(r.Context(), ResponseWriteError,
				HandlerNameKey.Field("openapi"),
				ErrorKey.Field(err.Error()),
			)
		}
	})

	// Docs handler at /docs
	e.mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
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

		if _, err := w.Write([]byte(html)); err != nil {
			capitan.Warn(r.Context(), ResponseWriteError,
				HandlerNameKey.Field("docs"),
				ErrorKey.Field(err.Error()),
			)
		}
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

// Start begins listening for HTTP requests on the given host and port.
// Use an empty string for host to bind to all interfaces.
// This method blocks until the server is shutdown.
func (e *Engine) Start(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	// Create HTTP server
	e.server = &http.Server{
		Addr:         addr,
		Handler:      e.mux,
		ReadTimeout:  e.config.ReadTimeout,
		WriteTimeout: e.config.WriteTimeout,
		IdleTimeout:  e.config.IdleTimeout,
		TLSConfig:    e.tlsConfig,
	}

	// Emit engine starting event
	capitan.Info(e.ctx, EngineStarting,
		AddressKey.Field(addr),
		TLSEnabledKey.Field(e.tlsConfig != nil),
	)

	var err error
	if e.tlsConfig != nil {
		// Certificates are provided via TLSConfig, so cert/key file args are empty.
		err = e.server.ListenAndServeTLS("", "")
	} else {
		err = e.server.ListenAndServe()
	}
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
