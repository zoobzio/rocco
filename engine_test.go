package rocco

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
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
	if engine.metrics == nil {
		t.Error("metrics registry not initialized")
	}
	if engine.tracer == nil {
		t.Error("tracer not initialized")
	}
	if engine.hooks == nil {
		t.Error("hooks not initialized")
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

	engine.Use(middleware)

	// Register a simple handler
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	)
	engine.Register(handler)

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

	engine.Register(handler)

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

	engine.Register(handler1, handler2)

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
	engine.Register(handler)

	// Register OpenAPI handler
	info := Info{
		Title:   "Test API",
		Version: "1.0.0",
	}
	engine.RegisterOpenAPIHandler("/openapi.json", info)

	// Test OpenAPI endpoint
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected content-type 'application/json', got %q", w.Header().Get("Content-Type"))
	}

	var spec OpenAPI
	err := json.Unmarshal(w.Body.Bytes(), &spec)
	if err != nil {
		t.Fatalf("failed to parse OpenAPI spec: %v", err)
	}
	if spec.Info.Title != "Test API" {
		t.Errorf("expected title 'Test API', got %q", spec.Info.Title)
	}
}

func TestEngine_Metrics(t *testing.T) {
	engine := NewEngine(nil)

	metrics := engine.Metrics()
	if metrics == nil {
		t.Error("expected metrics registry, got nil")
	}
}

func TestEngine_Tracer(t *testing.T) {
	engine := NewEngine(nil)

	tracer := engine.Tracer()
	if tracer == nil {
		t.Error("expected tracer, got nil")
	}
}

func TestEngine_Hooks(t *testing.T) {
	engine := NewEngine(nil)

	hooks := engine.Hooks()
	if hooks == nil {
		t.Error("expected hooks registry, got nil")
	}
}

func TestEngine_OnRequestReceived(t *testing.T) {
	engine := NewEngine(nil)

	called := false
	err := engine.OnRequestReceived(func(ctx context.Context, event *Event) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Register and call a handler to trigger the hook
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	)
	engine.Register(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	// Wait for async hooks to complete
	engine.Hooks().Close()

	if !called {
		t.Error("OnRequestReceived hook was not called")
	}
}

func TestEngine_OnRequestCompleted(t *testing.T) {
	engine := NewEngine(nil)

	called := false
	err := engine.OnRequestCompleted(func(ctx context.Context, event *Event) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	)
	engine.Register(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	// Wait for async hooks to complete
	engine.Hooks().Close()

	if !called {
		t.Error("OnRequestCompleted hook was not called")
	}
}

func TestEngine_OnRequestRejected(t *testing.T) {
	engine := NewEngine(nil)

	called := false
	err := engine.OnRequestRejected(func(ctx context.Context, event *Event) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create handler that returns an error
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, ErrInternalServer
		},
	)
	engine.Register(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	// Wait for async hooks to complete
	engine.Hooks().Close()

	if !called {
		t.Error("OnRequestRejected hook was not called")
	}
}

func TestEngine_OnShutdownStarted(t *testing.T) {
	engine := NewEngine(nil)

	var called atomic.Bool
	err := engine.OnShutdownStarted(func(ctx context.Context, event *Event) error {
		called.Store(true)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start server in background
	go func() {
		engine.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	engine.Shutdown(ctx)

	if !called.Load() {
		t.Error("OnShutdownStarted hook was not called")
	}
}

func TestEngine_OnShutdownComplete(t *testing.T) {
	engine := NewEngine(nil)

	var called atomic.Bool
	err := engine.OnShutdownComplete(func(ctx context.Context, event *Event) error {
		called.Store(true)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start server in background
	go func() {
		engine.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	engine.Shutdown(ctx)

	// Wait for async hooks to complete
	engine.Hooks().Close()

	if !called.Load() {
		t.Error("OnShutdownComplete hook was not called")
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
