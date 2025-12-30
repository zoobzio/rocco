package rocco

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zoobzio/openapi"
)

// newTestEngine creates an engine for testing without authentication.
func newTestEngine() *Engine {
	return NewEngine("localhost", 8080, nil)
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine("localhost", 8080, nil)

	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
	if engine.config.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %s", engine.config.Host)
	}
	if engine.config.Port != 8080 {
		t.Errorf("expected port 8080, got %d", engine.config.Port)
	}
	if engine.server == nil {
		t.Error("HTTP server not initialized")
	}
	if engine.mux == nil {
		t.Error("Chi router not initialized")
	}
}

func TestNewEngine_NilConfig(t *testing.T) {
	engine := newTestEngine()

	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
	if engine.config.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", engine.config.Port)
	}
}

func TestEngine_Router(t *testing.T) {
	engine := newTestEngine()

	router := engine.Router()
	if router == nil {
		t.Fatal("expected *http.ServeMux, got nil")
	}

	// Verify it's the same instance
	if router != engine.mux {
		t.Error("Router() returned different instance than internal mux")
	}
}

func TestEngine_Use(t *testing.T) {
	engine := newTestEngine()

	middlewareCalled := false
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalled = true
			next.ServeHTTP(w, r)
		})
	}

	engine.WithMiddleware(middleware)

	// Register a simple handler
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	)
	engine.WithHandlers(handler)

	// Test middleware is called
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if !middlewareCalled {
		t.Error("middleware was not called")
	}
}

func TestEngine_Register(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "test"}, nil
		},
	)

	engine.WithHandlers(handler)

	if len(engine.handlers) != 1 {
		t.Errorf("expected 1 handler registered, got %d", len(engine.handlers))
	}

	// Test handler is accessible via router
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEngine_RegisterMultiple(t *testing.T) {
	engine := newTestEngine()

	handler1 := NewHandler[NoBody, testOutput](
		"test1",
		"GET",
		"/test1",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "test1"}, nil
		},
	)

	handler2 := NewHandler[NoBody, testOutput](
		"test2",
		"POST",
		"/test2",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "test2"}, nil
		},
	)

	engine.WithHandlers(handler1, handler2)

	if len(engine.handlers) != 2 {
		t.Errorf("expected 2 handlers registered, got %d", len(engine.handlers))
	}
}

func TestEngine_RegisterOpenAPIHandler(t *testing.T) {
	engine := newTestEngine()

	// Register a test handler first
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	)
	engine.WithHandlers(handler)

	// Test default OpenAPI endpoint at /openapi
	req := httptest.NewRequest("GET", "/openapi", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected content-type 'application/json', got %q", w.Header().Get("Content-Type"))
	}

	var spec openapi.OpenAPI
	err := json.Unmarshal(w.Body.Bytes(), &spec)
	if err != nil {
		t.Fatalf("failed to parse OpenAPI spec: %v", err)
	}
	if spec.Info.Title != "API" {
		t.Errorf("expected title 'API', got %q", spec.Info.Title)
	}
	if spec.Info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", spec.Info.Version)
	}
}

func TestEngine_Shutdown(t *testing.T) {
	engine := NewEngine("localhost", 0, nil) // Use random port

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- engine.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := engine.Shutdown(ctx)
	if err != nil {
		t.Errorf("unexpected shutdown error: %v", err)
	}

	// Wait for server to finish
	select {
	case err := <-serverErr:
		// http.ErrServerClosed is expected and not an error
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}
}

func TestEngine_Register_HandlerMiddleware(t *testing.T) {
	engine := newTestEngine()

	var middlewareCalled bool
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalled = true
			w.Header().Set("X-Middleware", "called")
			next.ServeHTTP(w, r)
		})
	}

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithMiddleware(middleware)

	engine.WithHandlers(handler)

	// Test that handler middleware is applied
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if !middlewareCalled {
		t.Error("handler middleware was not called")
	}
	if w.Header().Get("X-Middleware") != "called" {
		t.Error("middleware header not set")
	}
	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEngine_Register_HandlerMiddlewareOrder(t *testing.T) {
	engine := newTestEngine()

	var callOrder []string

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "mw1")
			next.ServeHTTP(w, r)
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "mw2")
			next.ServeHTTP(w, r)
		})
	}

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			callOrder = append(callOrder, "handler")
			return testOutput{Message: "OK"}, nil
		},
	).WithMiddleware(mw1, mw2)

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "mw1" || callOrder[1] != "mw2" || callOrder[2] != "handler" {
		t.Errorf("expected order [mw1, mw2, handler], got %v", callOrder)
	}
}

func TestEngine_Register_HandlerAndEngineMiddleware(t *testing.T) {
	engine := newTestEngine()

	var callOrder []string

	// Engine-level middleware
	engineMw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "engine")
			next.ServeHTTP(w, r)
		})
	}
	engine.WithMiddleware(engineMw)

	// Handler-level middleware
	handlerMw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "handler-mw")
			next.ServeHTTP(w, r)
		})
	}

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			callOrder = append(callOrder, "handler")
			return testOutput{Message: "OK"}, nil
		},
	).WithMiddleware(handlerMw)

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	// Engine middleware runs first, then handler middleware, then handler
	if callOrder[0] != "engine" || callOrder[1] != "handler-mw" || callOrder[2] != "handler" {
		t.Errorf("expected order [engine, handler-mw, handler], got %v", callOrder)
	}
}

func TestEngine_Register_NoHandlerMiddleware(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	)

	// Should not panic with no middleware
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEngine_AdaptHandler_ErrorPath(t *testing.T) {
	engine := newTestEngine()

	// Handler that returns an error (using errors.New to trigger the real error path)
	errorHandler := NewHandler[NoBody, testOutput](
		"error-handler",
		"GET",
		"/error",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, errors.New("unexpected error")
		},
	)

	engine.WithHandlers(errorHandler)

	req := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	// Should get 500 status for internal server error
	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestEngine_WithSpec(t *testing.T) {
	engine := newTestEngine()

	customSpec := &EngineSpec{
		Info: openapi.Info{
			Title:       "Custom API",
			Version:     "2.0.0",
			Description: "Custom description",
		},
		Tags: []openapi.Tag{
			{Name: "custom", Description: "Custom tag"},
		},
	}

	result := engine.WithSpec(customSpec)

	// Should return engine for chaining
	if result != engine {
		t.Error("WithSpec should return the engine for chaining")
	}

	// Verify spec was set
	if engine.spec.Info.Title != "Custom API" {
		t.Errorf("expected title 'Custom API', got %q", engine.spec.Info.Title)
	}
	if engine.spec.Info.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", engine.spec.Info.Version)
	}
	if len(engine.spec.Tags) != 1 || engine.spec.Tags[0].Name != "custom" {
		t.Error("expected custom tag to be set")
	}
}

func TestEngine_WithTag(t *testing.T) {
	engine := newTestEngine()

	// Add a new tag
	result := engine.WithTag("users", "User management endpoints")

	// Should return engine for chaining
	if result != engine {
		t.Error("WithTag should return the engine for chaining")
	}

	// Verify tag was added
	found := false
	for _, tag := range engine.spec.Tags {
		if tag.Name == "users" && tag.Description == "User management endpoints" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'users' tag to be added")
	}
}

func TestEngine_WithTag_UpdateExisting(t *testing.T) {
	engine := newTestEngine()

	// Add initial tag
	engine.WithTag("users", "Initial description")

	// Update the same tag
	engine.WithTag("users", "Updated description")

	// Count tags with name "users" - should be only 1
	count := 0
	var desc string
	for _, tag := range engine.spec.Tags {
		if tag.Name == "users" {
			count++
			desc = tag.Description
		}
	}

	if count != 1 {
		t.Errorf("expected 1 'users' tag, got %d", count)
	}
	if desc != "Updated description" {
		t.Errorf("expected 'Updated description', got %q", desc)
	}
}

func TestEngine_WithTag_MultipleTags(t *testing.T) {
	engine := newTestEngine()

	engine.WithTag("users", "User endpoints").
		WithTag("orders", "Order endpoints").
		WithTag("products", "Product endpoints")

	if len(engine.spec.Tags) < 3 {
		t.Errorf("expected at least 3 tags, got %d", len(engine.spec.Tags))
	}

	// Verify all tags exist
	tagNames := make(map[string]bool)
	for _, tag := range engine.spec.Tags {
		tagNames[tag.Name] = true
	}

	for _, expected := range []string{"users", "orders", "products"} {
		if !tagNames[expected] {
			t.Errorf("missing tag %q", expected)
		}
	}
}

// Tests for default handlers (/openapi and /docs)

func TestEngine_DefaultHandlers_OpenAPI(t *testing.T) {
	engine := newTestEngine()

	// Register a handler to trigger default handlers setup
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	)
	engine.WithHandlers(handler)

	// Test /openapi endpoint
	req := httptest.NewRequest("GET", "/openapi", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Verify it's valid JSON
	var spec map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	// Verify OpenAPI structure
	if spec["openapi"] != "3.1.0" {
		t.Errorf("expected openapi version '3.1.0', got %v", spec["openapi"])
	}
}

func TestEngine_DefaultHandlers_OpenAPI_Cached(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	)
	engine.WithHandlers(handler)

	// First request
	req1 := httptest.NewRequest("GET", "/openapi", nil)
	w1 := httptest.NewRecorder()
	engine.mux.ServeHTTP(w1, req1)

	// Second request (should return cached)
	req2 := httptest.NewRequest("GET", "/openapi", nil)
	w2 := httptest.NewRecorder()
	engine.mux.ServeHTTP(w2, req2)

	// Both should return the same content
	if w1.Body.String() != w2.Body.String() {
		t.Error("expected cached OpenAPI spec to be identical")
	}
}

func TestEngine_DefaultHandlers_Docs(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	)
	engine.WithHandlers(handler)

	// Test /docs endpoint
	req := httptest.NewRequest("GET", "/docs", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/html; charset=utf-8', got %q", ct)
	}

	// Verify it contains Scalar API reference
	body := w.Body.String()
	if !contains(body, "api-reference") {
		t.Error("expected docs to contain 'api-reference'")
	}
	if !contains(body, "scalar") {
		t.Error("expected docs to contain 'scalar'")
	}
}

// Tests for authentication middleware

func TestEngine_AuthMiddleware_Success(t *testing.T) {
	// Create engine with auth extractor
	engine := NewEngine("localhost", 8080, func(_ context.Context, r *http.Request) (Identity, error) {
		// Extract token from header
		token := r.Header.Get("Authorization")
		if token == "Bearer valid-token" {
			return &testIdentity{id: "user-123", tenantID: "tenant-456"}, nil
		}
		return nil, errors.New("invalid token")
	})

	handler := NewHandler[NoBody, testOutput](
		"protected",
		"GET",
		"/protected",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "Hello " + req.Identity.ID()}, nil
		},
	).WithAuthentication()

	engine.WithHandlers(handler)

	// Request with valid token
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp testOutput
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "Hello user-123" {
		t.Errorf("expected 'Hello user-123', got %q", resp.Message)
	}
}

func TestEngine_AuthMiddleware_Failure(t *testing.T) {
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return nil, errors.New("authentication failed")
	})

	handler := NewHandler[NoBody, testOutput](
		"protected",
		"GET",
		"/protected",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithAuthentication()

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

// Tests for authorization middleware edge cases

func TestEngine_AuthzMiddleware_InsufficientScope(t *testing.T) {
	// Engine with auth that returns identity without required scope
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			id:       "user-1",
			tenantID: "tenant-1",
			scopes:   []string{"read"}, // Has read but not admin
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"scoped",
		"GET",
		"/scoped",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithScopes("admin") // Requires admin scope

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/scoped", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestEngine_AuthzMiddleware_InsufficientRole(t *testing.T) {
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			id:       "user-1",
			tenantID: "tenant-1",
			roles:    []string{"user"}, // Has user but not admin
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"role-protected",
		"GET",
		"/admin",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithRoles("admin") // Requires admin role

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

// Tests for usage limit middleware edge cases

func TestEngine_UsageLimitMiddleware_LimitExceeded(t *testing.T) {
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			id:       "user-1",
			tenantID: "tenant-1",
			stats:    map[string]int{"requests": 150}, // Over limit
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"limited",
		"GET",
		"/limited",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithUsageLimit("requests", func(_ Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/limited", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", w.Code)
	}
}

func TestEngine_UsageLimitMiddleware_NilStats(t *testing.T) {
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			id:       "user-1",
			tenantID: "tenant-1",
			stats:    nil, // No stats
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"limited",
		"GET",
		"/limited",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithUsageLimit("requests", func(_ Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/limited", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	// Should succeed since nil stats means 0 usage
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEngine_UsageLimitMiddleware_MissingStatKey(t *testing.T) {
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			id:       "user-1",
			tenantID: "tenant-1",
			stats:    map[string]int{"other_key": 50}, // Different key
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"limited",
		"GET",
		"/limited",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithUsageLimit("requests", func(_ Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/limited", nil)
	w := httptest.NewRecorder()
	engine.mux.ServeHTTP(w, req)

	// Should succeed since missing key means 0 usage
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
