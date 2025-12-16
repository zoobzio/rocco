package rocco

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
)

// TestMain sets up capitan in sync mode for all tests.
func TestMain(m *testing.M) {
	// Configure capitan before any tests run (before default instance is created).
	capitan.Configure(capitan.WithSyncMode())
	os.Exit(m.Run())
}

// setupSyncMode is a no-op helper for clarity in tests.
func setupSyncMode(t *testing.T) {
	t.Helper()
	// Sync mode already configured in TestMain.
}

func TestEvents_EngineCreated(t *testing.T) {
	setupSyncMode(t)

	var received bool
	var host string
	var port int

	listener := capitan.Hook(EngineCreated, func(_ context.Context, e *capitan.Event) {
		received = true
		host, _ = HostKey.From(e)
		port, _ = PortKey.From(e)
	})
	defer listener.Close()

	_ = NewEngine("localhost", 9000, nil)

	if !received {
		t.Error("EngineCreated not emitted")
	}
	if host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", host)
	}
	if port != 9000 {
		t.Errorf("expected port 9000, got %d", port)
	}
}

func TestEvents_HandlerRegistered(t *testing.T) {
	setupSyncMode(t)

	var received bool
	var handlerName, method, path string

	listener := capitan.Hook(HandlerRegistered, func(_ context.Context, e *capitan.Event) {
		received = true
		handlerName, _ = HandlerNameKey.From(e)
		method, _ = MethodKey.From(e)
		path, _ = PathKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"test-handler",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	)
	engine.WithHandlers(handler)

	if !received {
		t.Error("HandlerRegistered not emitted")
	}
	if handlerName != "test-handler" {
		t.Errorf("expected handler name 'test-handler', got %q", handlerName)
	}
	if method != "GET" {
		t.Errorf("expected method 'GET', got %q", method)
	}
	if path != "/test" {
		t.Errorf("expected path '/test', got %q", path)
	}
}

func TestEvents_RequestLifecycle_Success(t *testing.T) {
	setupSyncMode(t)

	var receivedCount int
	var requestReceived, requestCompleted bool
	var reqMethod, reqPath, reqHandler string

	listener1 := capitan.Hook(RequestReceived, func(_ context.Context, e *capitan.Event) {
		receivedCount++
		requestReceived = true
		reqMethod, _ = MethodKey.From(e)
		reqPath, _ = PathKey.From(e)
		reqHandler, _ = HandlerNameKey.From(e)
	})
	defer listener1.Close()

	listener2 := capitan.Hook(RequestCompleted, func(_ context.Context, e *capitan.Event) {
		receivedCount++
		requestCompleted = true
	})
	defer listener2.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"test-handler",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "success"}, nil
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !requestReceived {
		t.Error("RequestReceived not emitted")
	}
	if !requestCompleted {
		t.Error("RequestCompleted not emitted")
	}
	if reqMethod != "GET" {
		t.Errorf("expected method 'GET', got %q", reqMethod)
	}
	if reqPath != "/test" {
		t.Errorf("expected path '/test', got %q", reqPath)
	}
	if reqHandler != "test-handler" {
		t.Errorf("expected handler 'test-handler', got %q", reqHandler)
	}
}

func TestEvents_RequestLifecycle_Failed(t *testing.T) {
	setupSyncMode(t)

	var requestFailed bool
	var errorMsg string

	listener := capitan.Hook(RequestFailed, func(_ context.Context, e *capitan.Event) {
		requestFailed = true
		errorMsg, _ = ErrorKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"failing-handler",
		"GET",
		"/fail",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, errors.New("something went wrong")
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/fail", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !requestFailed {
		t.Error("RequestFailed not emitted")
	}
	if !strings.Contains(errorMsg, "something went wrong") {
		t.Errorf("expected error to contain 'something went wrong', got %q", errorMsg)
	}
}

func TestEvents_HandlerExecuting(t *testing.T) {
	setupSyncMode(t)

	var handlerExecuting bool
	var handlerName string

	listener := capitan.Hook(HandlerExecuting, func(_ context.Context, e *capitan.Event) {
		handlerExecuting = true
		handlerName, _ = HandlerNameKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"exec-handler",
		"GET",
		"/exec",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/exec", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !handlerExecuting {
		t.Error("HandlerExecuting not emitted")
	}
	if handlerName != "exec-handler" {
		t.Errorf("expected handler 'exec-handler', got %q", handlerName)
	}
}

func TestEvents_HandlerSuccess(t *testing.T) {
	setupSyncMode(t)

	var handlerSuccess bool
	var statusCode int

	listener := capitan.Hook(HandlerSuccess, func(_ context.Context, e *capitan.Event) {
		handlerSuccess = true
		statusCode, _ = StatusCodeKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"success-handler",
		"POST",
		"/success",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "created"}, nil
		},
	).WithSuccessStatus(http.StatusCreated)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("POST", "/success", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !handlerSuccess {
		t.Error("HandlerSuccess not emitted")
	}
	if statusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", statusCode)
	}
}

func TestEvents_HandlerError(t *testing.T) {
	setupSyncMode(t)

	var handlerError bool
	var errorMsg string

	listener := capitan.Hook(HandlerError, func(_ context.Context, e *capitan.Event) {
		handlerError = true
		errorMsg, _ = ErrorKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"error-handler",
		"GET",
		"/error",
		func(_ *Request[NoBody]) (testOutput, error) {
			// Use a plain error (not a rocco Error) to trigger HandlerError event
			return testOutput{}, errors.New("database connection failed")
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !handlerError {
		t.Error("HandlerError not emitted")
	}
	if errorMsg == "" {
		t.Error("expected error message")
	}
}

func TestEvents_HandlerSentinelError(t *testing.T) {
	setupSyncMode(t)

	var sentinelError bool
	var statusCode int
	var errorMsg string

	listener := capitan.Hook(HandlerSentinelError, func(_ context.Context, e *capitan.Event) {
		sentinelError = true
		statusCode, _ = StatusCodeKey.From(e)
		errorMsg, _ = ErrorKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"sentinel-handler",
		"GET",
		"/sentinel",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, ErrNotFound
		},
	).WithErrors(ErrNotFound)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/sentinel", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !sentinelError {
		t.Error("HandlerSentinelError not emitted")
	}
	if statusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", statusCode)
	}
	if !strings.Contains(errorMsg, "not found") {
		t.Errorf("expected error to contain 'not found', got %q", errorMsg)
	}
}

func TestEvents_HandlerUndeclaredSentinel(t *testing.T) {
	setupSyncMode(t)

	var undeclaredSentinel bool
	var statusCode int

	listener := capitan.Hook(HandlerUndeclaredSentinel, func(_ context.Context, e *capitan.Event) {
		undeclaredSentinel = true
		statusCode, _ = StatusCodeKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"undeclared-handler",
		"GET",
		"/undeclared",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, ErrNotFound
		},
	) // Missing .WithErrorCodes(404)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/undeclared", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !undeclaredSentinel {
		t.Error("HandlerUndeclaredSentinel not emitted")
	}
	if statusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", statusCode)
	}
}

func TestEvents_RequestBodyParseError(t *testing.T) {
	setupSyncMode(t)

	var parseError bool
	var errorMsg string

	listener := capitan.Hook(RequestBodyParseError, func(_ context.Context, e *capitan.Event) {
		parseError = true
		errorMsg, _ = ErrorKey.From(e)
	})
	defer listener.Close()

	engine := newTestEngine()
	handler := NewHandler[testInput, testOutput](
		"parse-handler",
		"POST",
		"/parse",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	)
	engine.WithHandlers(handler)

	// Invalid JSON
	req := httptest.NewRequest("POST", "/parse", bytes.NewBufferString("{invalid json"))
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !parseError {
		t.Error("RequestBodyParseError not emitted")
	}
	if errorMsg == "" {
		t.Error("expected error message")
	}
}

func TestEvents_RequestValidationInputFailed(t *testing.T) {
	setupSyncMode(t)

	var validationFailed bool
	var errorMsg string

	listener := capitan.Hook(RequestValidationInputFailed, func(_ context.Context, e *capitan.Event) {
		validationFailed = true
		errorMsg, _ = ErrorKey.From(e)
	})
	defer listener.Close()

	type validatedInput struct {
		Email string `json:"email" validate:"required,email"`
	}

	engine := newTestEngine()
	handler := NewHandler[validatedInput, testOutput](
		"validation-handler",
		"POST",
		"/validate",
		func(_ *Request[validatedInput]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	)
	engine.WithHandlers(handler)

	// Invalid email
	body, _ := json.Marshal(map[string]string{"email": "not-an-email"})
	req := httptest.NewRequest("POST", "/validate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !validationFailed {
		t.Error("RequestValidationInputFailed not emitted")
	}
	if errorMsg == "" {
		t.Error("expected error message")
	}
}

func TestEvents_RequestValidationOutputFailed(t *testing.T) {
	setupSyncMode(t)

	var validationFailed bool
	var errorMsg string

	listener := capitan.Hook(RequestValidationOutputFailed, func(_ context.Context, e *capitan.Event) {
		validationFailed = true
		errorMsg, _ = ErrorKey.From(e)
	})
	defer listener.Close()

	type validatedOutput struct {
		Email string `json:"email" validate:"required,email"`
	}

	engine := newTestEngine()
	handler := NewHandler[NoBody, validatedOutput](
		"output-validation-handler",
		"GET",
		"/output-validate",
		func(_ *Request[NoBody]) (validatedOutput, error) {
			// Return invalid email
			return validatedOutput{Email: "not-an-email"}, nil
		},
	).WithOutputValidation() // Opt-in to output validation for this test
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/output-validate", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if !validationFailed {
		t.Error("RequestValidationOutputFailed not emitted")
	}
	if errorMsg == "" {
		t.Error("expected error message")
	}
}

func TestEvents_EngineShutdown(t *testing.T) {
	setupSyncMode(t)

	var shutdownStarted, shutdownComplete bool
	var graceful bool

	listener1 := capitan.Hook(EngineShutdownStarted, func(_ context.Context, e *capitan.Event) {
		shutdownStarted = true
	})
	defer listener1.Close()

	listener2 := capitan.Hook(EngineShutdownComplete, func(_ context.Context, e *capitan.Event) {
		shutdownComplete = true
		graceful, _ = GracefulKey.From(e)
	})
	defer listener2.Close()

	engine := NewEngine("localhost", 0, nil) // Random port

	// Start server in background
	go func() {
		_ = engine.Start()
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	ctx, ctxCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer ctxCancel()

	err := engine.Shutdown(ctx)
	if err != nil {
		t.Errorf("unexpected shutdown error: %v", err)
	}

	if !shutdownStarted {
		t.Error("EngineShutdownStarted not emitted")
	}
	if !shutdownComplete {
		t.Error("EngineShutdownComplete not emitted")
	}
	if !graceful {
		t.Error("expected graceful shutdown")
	}
}
