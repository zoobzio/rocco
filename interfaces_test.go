package rocco

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Mock implementation of Endpoint for testing
type mockHandler struct {
	processFunc func(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error)
	spec        HandlerSpec
}

func (m *mockHandler) Process(ctx context.Context, r *http.Request, w http.ResponseWriter) (int, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, r, w)
	}
	return http.StatusOK, nil
}

func (m *mockHandler) Spec() HandlerSpec {
	return m.spec
}

func (*mockHandler) Close() error { return nil }

func (*mockHandler) Middleware() []func(http.Handler) http.Handler {
	return nil
}

func TestEndpoint_Interface(t *testing.T) {
	handler := &mockHandler{
		spec: HandlerSpec{
			Name:           "test-handler",
			Method:         "GET",
			Path:           "/test",
			Summary:        "Test handler",
			Description:    "A test handler",
			Tags:           []string{"test"},
			PathParams:     []string{"id"},
			QueryParams:    []string{"page"},
			InputTypeName:  "MockInput",
			OutputTypeName: "MockOutput",
			SuccessStatus:  200,
			ErrorCodes:     []int{404, 500},
		},
	}

	// Test spec method
	spec := handler.Spec()
	if spec.Name != "test-handler" {
		t.Errorf("expected name 'test-handler', got %q", spec.Name)
	}
	if spec.Method != "GET" {
		t.Errorf("expected method 'GET', got %q", spec.Method)
	}
	if spec.Path != "/test" {
		t.Errorf("expected path '/test', got %q", spec.Path)
	}
	if spec.Summary != "Test handler" {
		t.Errorf("expected summary 'Test handler', got %q", spec.Summary)
	}
	if spec.Description != "A test handler" {
		t.Errorf("expected description 'A test handler', got %q", spec.Description)
	}
	if len(spec.Tags) != 1 || spec.Tags[0] != "test" {
		t.Errorf("expected tags ['test'], got %v", spec.Tags)
	}
	if len(spec.PathParams) != 1 || spec.PathParams[0] != "id" {
		t.Errorf("expected path params ['id'], got %v", spec.PathParams)
	}
	if len(spec.QueryParams) != 1 || spec.QueryParams[0] != "page" {
		t.Errorf("expected query params ['page'], got %v", spec.QueryParams)
	}
	if spec.SuccessStatus != 200 {
		t.Errorf("expected success status 200, got %d", spec.SuccessStatus)
	}
	if len(spec.ErrorCodes) != 2 {
		t.Errorf("expected 2 error codes, got %d", len(spec.ErrorCodes))
	}
	if spec.InputTypeName != "MockInput" {
		t.Errorf("expected input type 'MockInput', got %q", spec.InputTypeName)
	}
	if spec.OutputTypeName != "MockOutput" {
		t.Errorf("expected output type 'MockOutput', got %q", spec.OutputTypeName)
	}
}

func TestEndpoint_Process(t *testing.T) {
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

func TestEndpoint_Close(t *testing.T) {
	handler := &mockHandler{}

	err := handler.Close()
	if err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}
