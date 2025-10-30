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

func TestNewEngine(t *testing.T) {
	config := DefaultConfig()
	engine := NewEngine(config)

	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
	if engine.config != config {
		t.Error("engine config not set correctly")
	}
	if engine.server == nil {
		t.Error("HTTP server not initialized")
	}
	if engine.chiRouter == nil {
		t.Error("Chi router not initialized")
	}
}

func TestNewEngine_NilConfig(t *testing.T) {
	engine := NewEngine(nil)

	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
	if engine.config.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", engine.config.Port)
	}
}

func TestEngine_Use(t *testing.T) {
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

	if !middlewareCalled {
		t.Error("middleware was not called")
	}
}

func TestEngine_Register(t *testing.T) {
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEngine_RegisterMultiple(t *testing.T) {
	engine := NewEngine(nil)

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
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

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
	config := DefaultConfig().WithPort(0) // Use random port
	engine := NewEngine(config)

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
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

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
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "mw1" || callOrder[1] != "mw2" || callOrder[2] != "handler" {
		t.Errorf("expected order [mw1, mw2, handler], got %v", callOrder)
	}
}

func TestEngine_Register_HandlerAndEngineMiddleware(t *testing.T) {
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	// Engine middleware runs first, then handler middleware, then handler
	if callOrder[0] != "engine" || callOrder[1] != "handler-mw" || callOrder[2] != "handler" {
		t.Errorf("expected order [engine, handler-mw, handler], got %v", callOrder)
	}
}

func TestEngine_Register_NoHandlerMiddleware(t *testing.T) {
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEngine_AdaptHandler_ErrorPath(t *testing.T) {
	engine := NewEngine(nil)

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
	engine.chiRouter.ServeHTTP(w, req)

	// Should get 500 status for internal server error
	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}
