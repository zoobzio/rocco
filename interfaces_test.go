package rocco

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Mock implementation of RouteHandler for testing
type mockHandler struct {
	processFunc func(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error)
	name        string
	method      string
	path        string
	summary     string
	description string
	tags        []string
	pathParams  []string
	queryParams []string
	successCode int
	errorCodes  []int
}

func (m *mockHandler) Process(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, r, w)
	}
	return http.StatusOK, nil
}

func (m *mockHandler) Name() string          { return m.name }
func (m *mockHandler) Method() string        { return m.method }
func (m *mockHandler) Path() string          { return m.path }
func (m *mockHandler) Summary() string       { return m.summary }
func (m *mockHandler) Description() string   { return m.description }
func (m *mockHandler) Tags() []string        { return m.tags }
func (m *mockHandler) PathParams() []string  { return m.pathParams }
func (m *mockHandler) QueryParams() []string { return m.queryParams }
func (m *mockHandler) SuccessStatus() int    { return m.successCode }
func (m *mockHandler) ErrorCodes() []int     { return m.errorCodes }
func (*mockHandler) InputSchema() *Schema    { return &Schema{Type: "object"} }
func (*mockHandler) OutputSchema() *Schema   { return &Schema{Type: "object"} }
func (*mockHandler) InputTypeName() string   { return "MockInput" }
func (*mockHandler) OutputTypeName() string  { return "MockOutput" }
func (*mockHandler) Close() error            { return nil }
func (*mockHandler) Middleware() []func(http.Handler) http.Handler {
	return nil
}

func TestRouteHandler_Interface(t *testing.T) {
	handler := &mockHandler{
		name:        "test-handler",
		method:      "GET",
		path:        "/test",
		summary:     "Test handler",
		description: "A test handler",
		tags:        []string{"test"},
		pathParams:  []string{"id"},
		queryParams: []string{"page"},
		successCode: 200,
		errorCodes:  []int{404, 500},
	}

	// Test all interface methods
	if handler.Name() != "test-handler" {
		t.Errorf("expected name 'test-handler', got %q", handler.Name())
	}
	if handler.Method() != "GET" {
		t.Errorf("expected method 'GET', got %q", handler.Method())
	}
	if handler.Path() != "/test" {
		t.Errorf("expected path '/test', got %q", handler.Path())
	}
	if handler.Summary() != "Test handler" {
		t.Errorf("expected summary 'Test handler', got %q", handler.Summary())
	}
	if handler.Description() != "A test handler" {
		t.Errorf("expected description 'A test handler', got %q", handler.Description())
	}
	if len(handler.Tags()) != 1 || handler.Tags()[0] != "test" {
		t.Errorf("expected tags ['test'], got %v", handler.Tags())
	}
	if len(handler.PathParams()) != 1 || handler.PathParams()[0] != "id" {
		t.Errorf("expected path params ['id'], got %v", handler.PathParams())
	}
	if len(handler.QueryParams()) != 1 || handler.QueryParams()[0] != "page" {
		t.Errorf("expected query params ['page'], got %v", handler.QueryParams())
	}
	if handler.SuccessStatus() != 200 {
		t.Errorf("expected success status 200, got %d", handler.SuccessStatus())
	}
	if len(handler.ErrorCodes()) != 2 {
		t.Errorf("expected 2 error codes, got %d", len(handler.ErrorCodes()))
	}
	if handler.InputTypeName() != "MockInput" {
		t.Errorf("expected input type 'MockInput', got %q", handler.InputTypeName())
	}
	if handler.OutputTypeName() != "MockOutput" {
		t.Errorf("expected output type 'MockOutput', got %q", handler.OutputTypeName())
	}
}

func TestRouteHandler_Process(t *testing.T) {
	called := false
	handler := &mockHandler{
		processFunc: func(_ context.Context, _ *http.Request, w http.ResponseWriter) (int, error) {
			called = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return http.StatusOK, nil
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	status, err := handler.Process(context.Background(), req, w)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, status)
	}
	if !called {
		t.Error("process function was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got %q", w.Body.String())
	}
}

func TestRouteHandler_Close(t *testing.T) {
	handler := &mockHandler{}

	err := handler.Close()
	if err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}
