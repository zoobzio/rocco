package rocco

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// errorReader is a reader that always returns an error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (e *errorReader) Close() error {
	return nil
}

type testInput struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type testOutput struct {
	Message string `json:"message"`
	Result  int    `json:"result"`
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test-handler",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	if handler.Name() != "test-handler" {
		t.Errorf("expected name 'test-handler', got %q", handler.Name())
	}
	if handler.Method() != "POST" {
		t.Errorf("expected method 'POST', got %q", handler.Method())
	}
	if handler.Path() != "/test" {
		t.Errorf("expected path '/test', got %q", handler.Path())
	}
	if handler.SuccessStatus() != 200 {
		t.Errorf("expected default success status 200, got %d", handler.SuccessStatus())
	}
	if handler.InputMeta.TypeName != "testInput" {
		t.Errorf("expected input type 'testInput', got %q", handler.InputMeta.TypeName)
	}
	if handler.OutputMeta.TypeName != "testOutput" {
		t.Errorf("expected output type 'testOutput', got %q", handler.OutputMeta.TypeName)
	}
}

func TestHandler_WithBuilderMethods(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test/{id}",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	).
		WithSummary("Test summary").
		WithDescription("Test description").
		WithTags("test", "example").
		WithSuccessStatus(201).
		WithPathParams("id").
		WithQueryParams("page", "limit").
		WithResponseHeaders(map[string]string{"X-Custom": "value"}).
		WithErrorCodes(400, 404)

	if handler.Summary() != "Test summary" {
		t.Errorf("expected summary 'Test summary', got %q", handler.Summary())
	}
	if handler.Description() != "Test description" {
		t.Errorf("expected description 'Test description', got %q", handler.Description())
	}
	if len(handler.Tags()) != 2 {
		t.Errorf("expected 2 tags, got %d", len(handler.Tags()))
	}
	if handler.SuccessStatus() != 201 {
		t.Errorf("expected success status 201, got %d", handler.SuccessStatus())
	}
	if len(handler.PathParams()) != 1 {
		t.Errorf("expected 1 path param, got %d", len(handler.PathParams()))
	}
	if len(handler.QueryParams()) != 2 {
		t.Errorf("expected 2 query params, got %d", len(handler.QueryParams()))
	}
	if len(handler.ErrorCodes()) != 2 {
		t.Errorf("expected 2 error codes, got %d", len(handler.ErrorCodes()))
	}
}

func TestHandler_Process_Success(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(req *Request[testInput]) (testOutput, error) {
			return testOutput{
				Message: fmt.Sprintf("Hello %s", req.Body.Name),
				Result:  req.Body.Count * 2,
			}, nil
		},
	)

	input := testInput{Name: "World", Count: 21}
	body, _ := json.Marshal(input)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var output testOutput
	json.Unmarshal(w.Body.Bytes(), &output)

	if output.Message != "Hello World" {
		t.Errorf("expected message 'Hello World', got %q", output.Message)
	}
	if output.Result != 42 {
		t.Errorf("expected result 42, got %d", output.Result)
	}
}

func TestHandler_Process_NoBody(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "No body"}, nil
		},
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandler_Process_InvalidJSON(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if w.Code != 422 {
		t.Errorf("expected status 422, got %d", w.Code)
	}
}

func TestHandler_Process_DeclaredSentinelError(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, ErrNotFound
		},
	).WithErrorCodes(404)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	// Declared sentinel should return nil error (successfully handled)
	if err != nil {
		t.Errorf("expected nil error for declared sentinel, got %v", err)
	}
	if w.Code != 404 {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "Not Found" {
		t.Errorf("expected error message 'Not Found', got %q", resp["error"])
	}
}

func TestHandler_Process_UndeclaredSentinelError(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, ErrNotFound
		},
	) // No WithErrorCodes() - undeclared

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	// Undeclared sentinel should return error with 500
	if err == nil {
		t.Fatal("expected error for undeclared sentinel")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error should wrap ErrNotFound, got %v", err)
	}
	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandler_Process_RealError(t *testing.T) {
	testErr := errors.New("something broke")
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, testErr
		},
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err != testErr {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandler_ExtractParams_PathParams(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/users/{id}",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithPathParams("id")

	// Create Chi route context
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "123")

	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	req := httptest.NewRequest("GET", "/users/123", nil)

	params, err := handler.extractParams(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Path["id"] != "123" {
		t.Errorf("expected path param 'id' = '123', got %q", params.Path["id"])
	}
}

func TestHandler_ExtractParams_MissingPathParam(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/users/{id}",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithPathParams("id")

	req := httptest.NewRequest("GET", "/users/123", nil)

	_, err := handler.extractParams(context.Background(), req)

	if err == nil {
		t.Fatal("expected error for missing path param")
	}
}

func TestHandler_ExtractParams_QueryParams(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithQueryParams("page", "limit")

	req := httptest.NewRequest("GET", "/test?page=1&limit=10", nil)

	params, err := handler.extractParams(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Query["page"] != "1" {
		t.Errorf("expected query param 'page' = '1', got %q", params.Query["page"])
	}
	if params.Query["limit"] != "10" {
		t.Errorf("expected query param 'limit' = '10', got %q", params.Query["limit"])
	}
}

func TestHandler_ExtractParams_MissingQueryParam(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithQueryParams("page")

	req := httptest.NewRequest("GET", "/test", nil)

	params, err := handler.extractParams(context.Background(), req)

	// Missing query params should result in empty string, not error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Query["page"] != "" {
		t.Errorf("expected empty string for missing query param, got %q", params.Query["page"])
	}
}

func TestMapSentinelToStatus(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{ErrBadRequest, 400},
		{ErrUnauthorized, 401},
		{ErrForbidden, 403},
		{ErrNotFound, 404},
		{ErrConflict, 409},
		{ErrUnprocessableEntity, 422},
		{ErrTooManyRequests, 429},
		{errors.New("unknown"), 500},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v->%d", tt.err, tt.status), func(t *testing.T) {
			status := mapSentinelToStatus(tt.err)
			if status != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, status)
			}
		})
	}
}

func TestIsSentinelError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{ErrBadRequest, true},
		{ErrNotFound, true},
		{ErrUnauthorized, true},
		{errors.New("random error"), false},
		{nil, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.err), func(t *testing.T) {
			result := isSentinelError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHandler_ValidationInput(t *testing.T) {
	type validatedInput struct {
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age" validate:"required,min=18"`
	}

	handler := NewHandler[validatedInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[validatedInput]) (testOutput, error) {
			return testOutput{Message: "valid"}, nil
		},
	)

	// Test invalid input
	invalidInput := `{"email":"notanemail","age":15}`
	req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte(invalidInput)))
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected validation error")
	}
	if w.Code != 422 {
		t.Errorf("expected status 422, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["error"] != "Validation failed" {
		t.Errorf("expected 'Validation failed', got %v", response["error"])
	}
}

func TestHandler_ValidationOutput(t *testing.T) {
	type validatedOutput struct {
		Email string `json:"email" validate:"required,email"`
	}

	handler := NewHandler[NoBody, validatedOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (validatedOutput, error) {
			// Return invalid output
			return validatedOutput{Email: "notanemail"}, nil
		},
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected validation error")
	}
	if w.Code != 500 {
		t.Errorf("expected status 500 for output validation, got %d", w.Code)
	}
}

func TestHandler_Use(t *testing.T) {
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	if len(handler.Middleware()) != 1 {
		t.Errorf("expected 1 middleware, got %d", len(handler.Middleware()))
	}
}

func TestHandler_UseMultiple(t *testing.T) {
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithMiddleware(mw1, mw2)

	if len(handler.Middleware()) != 2 {
		t.Errorf("expected 2 middleware, got %d", len(handler.Middleware()))
	}
}

func TestHandler_Close(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	err := handler.Close()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestHandler_OutputSchema(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	schema := handler.OutputSchema()
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
	if schema.Type != "object" {
		t.Errorf("expected type 'object', got %q", schema.Type)
	}
}

func TestHandler_WithMaxBodySize(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithMaxBodySize(1024)

	if handler.maxBodySize != 1024 {
		t.Errorf("expected maxBodySize 1024, got %d", handler.maxBodySize)
	}
}

func TestHandler_Process_MaxBodySizeExceeded(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{Message: "Should not reach here"}, nil
		},
	).WithMaxBodySize(10) // Very small limit

	// Create body larger than limit
	largeBody := bytes.Repeat([]byte("a"), 100)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(largeBody))
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected error for body size exceeded")
	}
	if w.Code != 422 {
		t.Errorf("expected status 422, got %d", w.Code)
	}
}

func TestHandler_Process_BodyReadError(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	req := httptest.NewRequest("POST", "/test", nil)
	req.Body = io.NopCloser(&errorReader{})
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected error from body read")
	}
	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandler_Process_ResponseHeaders(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithResponseHeaders(map[string]string{
		"X-Custom-Header": "custom-value",
		"X-API-Version":   "1.0",
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	err := handler.Process(context.Background(), req, w)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Header().Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got %q", w.Header().Get("X-Custom-Header"))
	}
	if w.Header().Get("X-API-Version") != "1.0" {
		t.Errorf("expected X-API-Version '1.0', got %q", w.Header().Get("X-API-Version"))
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", w.Header().Get("Content-Type"))
	}
}
