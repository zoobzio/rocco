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
	"strings"
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

	spec := handler.Spec()
	if spec.Name != "test-handler" {
		t.Errorf("expected name 'test-handler', got %q", spec.Name)
	}
	if spec.Method != "POST" {
		t.Errorf("expected method 'POST', got %q", spec.Method)
	}
	if spec.Path != "/test" {
		t.Errorf("expected path '/test', got %q", spec.Path)
	}
	if spec.SuccessStatus != 200 {
		t.Errorf("expected default success status 200, got %d", spec.SuccessStatus)
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
		WithErrors(ErrBadRequest, ErrNotFound)

	spec := handler.Spec()
	if spec.Summary != "Test summary" {
		t.Errorf("expected summary 'Test summary', got %q", spec.Summary)
	}
	if spec.Description != "Test description" {
		t.Errorf("expected description 'Test description', got %q", spec.Description)
	}
	if len(spec.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(spec.Tags))
	}
	if spec.SuccessStatus != 201 {
		t.Errorf("expected success status 201, got %d", spec.SuccessStatus)
	}
	if len(spec.PathParams) != 1 {
		t.Errorf("expected 1 path param, got %d", len(spec.PathParams))
	}
	if len(spec.QueryParams) != 2 {
		t.Errorf("expected 2 query params, got %d", len(spec.QueryParams))
	}
	if len(spec.ErrorCodes) != 2 {
		t.Errorf("expected 2 error codes, got %d", len(spec.ErrorCodes))
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

	_, err := handler.Process(context.Background(), req, w)

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

	_, err := handler.Process(context.Background(), req, w)

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

	_, err := handler.Process(context.Background(), req, w)

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
	).WithErrors(ErrNotFound)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)

	// Declared sentinel should return nil error (successfully handled)
	if err != nil {
		t.Errorf("expected nil error for declared sentinel, got %v", err)
	}
	if w.Code != 404 {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "NOT_FOUND" {
		t.Errorf("expected error code 'NOT_FOUND', got %q", resp["code"])
	}
	if resp["message"] != "not found" {
		t.Errorf("expected error message 'not found', got %q", resp["message"])
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
	) // No WithErrors() - undeclared

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)

	// Undeclared sentinel should return error with 500
	if err == nil {
		t.Fatal("expected error for undeclared sentinel")
	}
	// Error message should indicate the undeclared error
	if !strings.Contains(err.Error(), "undeclared error") {
		t.Errorf("error should mention undeclared error, got %v", err)
	}
	if !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Errorf("error should mention NOT_FOUND, got %v", err)
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

	_, err := handler.Process(context.Background(), req, w)

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

func TestGetRoccoError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorDefinition
	}{
		{"ErrBadRequest", ErrBadRequest, ErrBadRequest},
		{"ErrNotFound", ErrNotFound, ErrNotFound},
		{"ErrNotFound with message", ErrNotFound.WithMessage("custom"), ErrNotFound},
		{"plain error", errors.New("random error"), nil},
		{"nil", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRoccoError(tt.err)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected non-nil, got nil")
				} else if result.Code() != tt.expected.Code() {
					t.Errorf("expected code %s, got %s", tt.expected.Code(), result.Code())
				}
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

	_, err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected validation error")
	}
	if w.Code != 422 {
		t.Errorf("expected status 422, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "VALIDATION_FAILED" {
		t.Errorf("expected code 'VALIDATION_FAILED', got %v", response["code"])
	}
	if response["message"] != "validation failed" {
		t.Errorf("expected message 'validation failed', got %v", response["message"])
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
	).WithOutputValidation() // Opt-in to output validation for this test

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)

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

	_, err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected error for body size exceeded")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", w.Code)
	}

	// Verify error response format
	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "PAYLOAD_TOO_LARGE" {
		t.Errorf("expected code 'PAYLOAD_TOO_LARGE', got %v", response["code"])
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

	_, err := handler.Process(context.Background(), req, w)

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

	_, err := handler.Process(context.Background(), req, w)

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

func TestHandler_WithAuthentication(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	// Verify default is false
	spec := handler.Spec()
	if spec.RequiresAuth {
		t.Error("expected RequiresAuth to be false by default")
	}

	// Test WithAuthentication sets RequiresAuth to true
	handler.WithAuthentication()
	spec = handler.Spec()
	if !spec.RequiresAuth {
		t.Error("expected RequiresAuth to be true after WithAuthentication()")
	}
}

func TestHandler_WithAuthentication_Chaining(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithAuthentication().WithSummary("Test")

	spec := handler.Spec()
	if !spec.RequiresAuth {
		t.Error("expected RequiresAuth to be true")
	}
	if spec.Summary != "Test" {
		t.Errorf("expected Summary 'Test', got %q", spec.Summary)
	}
}

func TestHandler_WithScopes(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithScopes("read", "write")

	spec := handler.Spec()

	// WithScopes should implicitly set RequiresAuth
	if !spec.RequiresAuth {
		t.Error("expected RequiresAuth to be true after WithScopes()")
	}

	// Check scopes are set correctly
	if len(spec.ScopeGroups) != 1 {
		t.Fatalf("expected 1 scope group, got %d", len(spec.ScopeGroups))
	}
	if len(spec.ScopeGroups[0]) != 2 {
		t.Errorf("expected 2 scopes in group, got %d", len(spec.ScopeGroups[0]))
	}
	if spec.ScopeGroups[0][0] != "read" || spec.ScopeGroups[0][1] != "write" {
		t.Errorf("expected scopes [read, write], got %v", spec.ScopeGroups[0])
	}
}

func TestHandler_WithScopes_MultipleGroups(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithScopes("read").WithScopes("admin")

	spec := handler.Spec()

	// Multiple calls should create AND logic (multiple groups)
	if len(spec.ScopeGroups) != 2 {
		t.Fatalf("expected 2 scope groups, got %d", len(spec.ScopeGroups))
	}
	if spec.ScopeGroups[0][0] != "read" {
		t.Errorf("expected first group to contain 'read', got %v", spec.ScopeGroups[0])
	}
	if spec.ScopeGroups[1][0] != "admin" {
		t.Errorf("expected second group to contain 'admin', got %v", spec.ScopeGroups[1])
	}
}

func TestHandler_WithScopes_EmptyDoesNothing(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithScopes()

	spec := handler.Spec()

	// Empty WithScopes should not set RequiresAuth or add groups
	if spec.RequiresAuth {
		t.Error("expected RequiresAuth to remain false with empty WithScopes()")
	}
	if len(spec.ScopeGroups) != 0 {
		t.Errorf("expected 0 scope groups, got %d", len(spec.ScopeGroups))
	}
}

func TestHandler_WithRoles(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithRoles("admin", "moderator")

	spec := handler.Spec()

	// WithRoles should implicitly set RequiresAuth
	if !spec.RequiresAuth {
		t.Error("expected RequiresAuth to be true after WithRoles()")
	}

	// Check roles are set correctly
	if len(spec.RoleGroups) != 1 {
		t.Fatalf("expected 1 role group, got %d", len(spec.RoleGroups))
	}
	if len(spec.RoleGroups[0]) != 2 {
		t.Errorf("expected 2 roles in group, got %d", len(spec.RoleGroups[0]))
	}
	if spec.RoleGroups[0][0] != "admin" || spec.RoleGroups[0][1] != "moderator" {
		t.Errorf("expected roles [admin, moderator], got %v", spec.RoleGroups[0])
	}
}

func TestHandler_WithRoles_MultipleGroups(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithRoles("admin").WithRoles("verified")

	spec := handler.Spec()

	// Multiple calls should create AND logic (multiple groups)
	if len(spec.RoleGroups) != 2 {
		t.Fatalf("expected 2 role groups, got %d", len(spec.RoleGroups))
	}
}

func TestHandler_WithRoles_EmptyDoesNothing(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithRoles()

	spec := handler.Spec()

	// Empty WithRoles should not set RequiresAuth or add groups
	if spec.RequiresAuth {
		t.Error("expected RequiresAuth to remain false with empty WithRoles()")
	}
	if len(spec.RoleGroups) != 0 {
		t.Errorf("expected 0 role groups, got %d", len(spec.RoleGroups))
	}
}
