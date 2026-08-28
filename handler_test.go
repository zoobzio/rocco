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

	"github.com/zoobz-io/check"
)

// failingValidatableInput implements Validatable and always fails using check.
type failingValidatableInput struct {
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func (f failingValidatableInput) Validate() error {
	// Use check.All to return a *check.Result that always fails
	return check.All(
		check.Required("", "test"), // Empty string always fails required
	)
}

// failingValidatableOutput implements Validatable and always fails using check.
type failingValidatableOutput struct {
	Email string `json:"email"`
}

func (f failingValidatableOutput) Validate() error {
	return check.All(
		check.Required("", "test"), // Empty string always fails required
	)
}

// plainErrorValidatableInput implements Validatable and returns a plain error.
// Used to test the fallback path in writeValidationErrorResponse.
type plainErrorValidatableInput struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (plainErrorValidatableInput) Validate() error {
	return errors.New("plain validation error")
}

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
	if spec.Response.Status != 200 {
		t.Errorf("expected default success status 200, got %d", spec.Response.Status)
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
	if spec.Response.Status != 201 {
		t.Errorf("expected success status 201, got %d", spec.Response.Status)
	}
	if len(spec.PathParams) != 1 {
		t.Errorf("expected 1 path param, got %d", len(spec.PathParams))
	}
	if len(spec.QueryParams) != 2 {
		t.Errorf("expected 2 query params, got %d", len(spec.QueryParams))
	}
	if len(handler.ErrorDefs()) != 2 {
		t.Errorf("expected 2 error definitions, got %d", len(handler.ErrorDefs()))
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

	// Create request with path value set
	req := httptest.NewRequest("GET", "/users/123", nil)
	req.SetPathValue("id", "123")

	spec := handler.Spec()
	params, err := extractParams(context.Background(), req, spec.PathParams, spec.QueryParams)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Path["id"] != "123" {
		t.Errorf("expected path param 'id' = '123', got %q", params.Path["id"])
	}
}

func TestHandler_ExtractParams_WildcardPathParam(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/content/{path...}",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithPathParams("path...")

	req := httptest.NewRequest("GET", "/content/src/lib/main.go", nil)
	req.SetPathValue("path", "src/lib/main.go")

	spec := handler.Spec()
	params, err := extractParams(context.Background(), req, spec.PathParams, spec.QueryParams)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Path["path..."] != "src/lib/main.go" {
		t.Errorf("expected path param 'path...' = 'src/lib/main.go', got %q", params.Path["path..."])
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

	spec := handler.Spec()
	_, err := extractParams(context.Background(), req, spec.PathParams, spec.QueryParams)

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

	spec := handler.Spec()
	params, err := extractParams(context.Background(), req, spec.PathParams, spec.QueryParams)

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

	spec := handler.Spec()
	params, err := extractParams(context.Background(), req, spec.PathParams, spec.QueryParams)

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
	// Use failingValidatableInput which implements Validatable and always fails
	handler := NewHandler[failingValidatableInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[failingValidatableInput]) (testOutput, error) {
			return testOutput{Message: "valid"}, nil
		},
	)

	// Test input validation with failing Validatable type
	input := `{"email":"test@example.com","age":25}`
	req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte(input)))
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
	// Use failingValidatableOutput which implements Validatable and always fails
	handler := NewHandler[NoBody, failingValidatableOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (failingValidatableOutput, error) {
			return failingValidatableOutput{Email: "test@example.com"}, nil
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

	if handler.Spec().Request.MaxBytes != 1024 {
		t.Errorf("expected Request.MaxBytes 1024, got %d", handler.Spec().Request.MaxBytes)
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

// failingWriter is a ResponseWriter that fails on Write.
type failingWriter struct {
	header http.Header
	code   int
}

func (f *failingWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func (f *failingWriter) WriteHeader(code int) {
	f.code = code
}

func TestHandler_Process_ResponseWriteFails(t *testing.T) {
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "hello"}, nil
		},
	)

	req := httptest.NewRequest("GET", "/test", nil)
	fw := &failingWriter{}

	// Should not panic - just emit warning event
	status, err := handler.Process(context.Background(), req, fw)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}
}

func TestHandler_Process_BodyCloseFails(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(r *Request[testInput]) (testOutput, error) {
			return testOutput{Message: r.Body.Name}, nil
		},
	)

	body := bytes.NewBufferString(`{"name":"test","count":1}`)
	req := httptest.NewRequest("POST", "/test", &failingCloser{Reader: body})
	w := httptest.NewRecorder()

	// Should not panic - just emit warning event
	status, err := handler.Process(context.Background(), req, w)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}
}

// failingCloser wraps a reader and fails on Close.
type failingCloser struct {
	io.Reader
}

func (f *failingCloser) Close() error {
	return errors.New("close failed")
}

// mockCodec is a test codec that uses a custom content type.
type mockCodec struct {
	contentType string
}

func (m mockCodec) ContentType() string {
	return m.contentType
}

func (m mockCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v) // Use JSON under the hood for simplicity
}

func (m mockCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func TestHandler_DefaultCodec(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	spec := handler.Spec()
	if spec.ContentType != "application/json" {
		t.Errorf("expected default content type 'application/json', got %q", spec.ContentType)
	}
}

func TestHandler_WithCodec(t *testing.T) {
	xmlCodec := mockCodec{contentType: "application/xml"}

	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithCodec(xmlCodec)

	spec := handler.Spec()
	if spec.ContentType != "application/xml" {
		t.Errorf("expected content type 'application/xml', got %q", spec.ContentType)
	}
}

func TestHandler_WithCodec_ResponseContentType(t *testing.T) {
	xmlCodec := mockCodec{contentType: "application/xml"}

	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "hello"}, nil
		},
	).WithCodec(xmlCodec)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Header().Get("Content-Type") != "application/xml" {
		t.Errorf("expected Content-Type 'application/xml', got %q", w.Header().Get("Content-Type"))
	}
}

func TestHandler_ApplyDefaultCodec(t *testing.T) {
	xmlCodec := mockCodec{contentType: "application/xml"}

	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	)

	// Simulate what engine does
	handler.applyDefaultCodec(xmlCodec)

	spec := handler.Spec()
	if spec.ContentType != "application/xml" {
		t.Errorf("expected content type 'application/xml', got %q", spec.ContentType)
	}
}

func TestHandler_ApplyDefaultCodec_DoesNotOverrideExplicit(t *testing.T) {
	xmlCodec := mockCodec{contentType: "application/xml"}
	yamlCodec := mockCodec{contentType: "application/yaml"}

	handler := NewHandler[testInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithCodec(xmlCodec) // Explicitly set

	// Engine tries to apply its default
	handler.applyDefaultCodec(yamlCodec)

	spec := handler.Spec()
	if spec.ContentType != "application/xml" {
		t.Errorf("expected content type 'application/xml' (explicit), got %q", spec.ContentType)
	}
}

func TestHandler_ValidationError_FallbackPath(t *testing.T) {
	// Test that plain errors (without ValidationDetails) are handled correctly.
	// Use plainErrorValidatableInput which implements Validatable and returns a plain error.
	handler := NewHandler[plainErrorValidatableInput, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[plainErrorValidatableInput]) (testOutput, error) {
			return testOutput{Message: "OK"}, nil
		},
	).WithErrors(ErrValidationFailed)

	body := `{"name":"test","count":1}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected validation error")
	}
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", w.Code)
	}

	// Verify the response contains the plain error message
	var resp map[string]any
	if jsonErr := json.Unmarshal(w.Body.Bytes(), &resp); jsonErr != nil {
		t.Fatalf("failed to unmarshal response: %v", jsonErr)
	}
	if resp["message"] != "plain validation error" {
		t.Errorf("expected message 'plain validation error', got %q", resp["message"])
	}
}

func TestGenerateHandlerName(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		expected string // prefix only, suffix is random
	}{
		{"GET", "/users", "get-users-"},
		{"POST", "/users", "post-users-"},
		{"GET", "/users/{id}", "get-users-id-"},
		{"PUT", "/users/{id}/profile", "put-users-id-profile-"},
		{"DELETE", "/", "delete-"},
		{"PATCH", "items/{itemId}", "patch-items-itemId-"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			name := generateHandlerName(tt.method, tt.path)
			if !strings.HasPrefix(name, tt.expected) {
				t.Errorf("expected prefix %q, got %q", tt.expected, name)
			}
			// Check suffix is 8 hex chars
			suffix := name[len(tt.expected):]
			if len(suffix) != 8 {
				t.Errorf("expected 8 char suffix, got %q (%d chars)", suffix, len(suffix))
			}
		})
	}
}

func TestGenerateHandlerName_Deterministic(t *testing.T) {
	// Same method+path must always produce the same name so operationIds and
	// telemetry keys stay stable across process restarts (issue #33).
	first := generateHandlerName("GET", "/test")
	for i := 0; i < 100; i++ {
		if got := generateHandlerName("GET", "/test"); got != first {
			t.Errorf("non-deterministic name: got %q, want %q", got, first)
		}
	}
}

func TestGenerateHandlerName_DistinctRoutes(t *testing.T) {
	// Different routes must produce different names, including routes whose
	// readable segments collide (a path param vs a literal of the same name).
	routes := []struct{ method, path string }{
		{"GET", "/users"},
		{"POST", "/users"},
		{"GET", "/users/{id}"},
		{"GET", "/users/id"}, // collides on segments with the line above
		{"PUT", "/users/{id}/profile"},
		{"DELETE", "/users/{id}"},
	}

	seen := make(map[string]string, len(routes))
	for _, r := range routes {
		name := generateHandlerName(r.method, r.path)
		if prev, dup := seen[name]; dup {
			t.Errorf("collision: %s %s and %s both produced %q", r.method, r.path, prev, name)
		}
		seen[name] = r.method + " " + r.path
	}
}

func TestGET(t *testing.T) {
	handler := GET[NoBody, testOutput]("/users/{id}", func(r *Request[NoBody]) (testOutput, error) {
		return testOutput{Message: "user"}, nil
	})

	spec := handler.Spec()
	if spec.Method != "GET" {
		t.Errorf("expected method GET, got %q", spec.Method)
	}
	if spec.Path != "/users/{id}" {
		t.Errorf("expected path '/users/{id}', got %q", spec.Path)
	}
	if !strings.HasPrefix(spec.Name, "get-users-id-") {
		t.Errorf("expected name prefix 'get-users-id-', got %q", spec.Name)
	}
}

func TestPOST(t *testing.T) {
	handler := POST[testInput, testOutput]("/users", func(r *Request[testInput]) (testOutput, error) {
		return testOutput{Message: r.Body.Name}, nil
	})

	spec := handler.Spec()
	if spec.Method != "POST" {
		t.Errorf("expected method POST, got %q", spec.Method)
	}
	if spec.Path != "/users" {
		t.Errorf("expected path '/users', got %q", spec.Path)
	}
}

func TestPUT(t *testing.T) {
	handler := PUT[testInput, testOutput]("/users/{id}", func(r *Request[testInput]) (testOutput, error) {
		return testOutput{}, nil
	})

	spec := handler.Spec()
	if spec.Method != "PUT" {
		t.Errorf("expected method PUT, got %q", spec.Method)
	}
}

func TestPATCH(t *testing.T) {
	handler := PATCH[testInput, testOutput]("/users/{id}", func(r *Request[testInput]) (testOutput, error) {
		return testOutput{}, nil
	})

	spec := handler.Spec()
	if spec.Method != "PATCH" {
		t.Errorf("expected method PATCH, got %q", spec.Method)
	}
}

func TestDELETE(t *testing.T) {
	handler := DELETE[NoBody, testOutput]("/users/{id}", func(r *Request[NoBody]) (testOutput, error) {
		return testOutput{}, nil
	})

	spec := handler.Spec()
	if spec.Method != "DELETE" {
		t.Errorf("expected method DELETE, got %q", spec.Method)
	}
}

func TestWithName(t *testing.T) {
	handler := GET[NoBody, testOutput]("/users", func(r *Request[NoBody]) (testOutput, error) {
		return testOutput{}, nil
	}).WithName("list-all-users")

	spec := handler.Spec()
	if spec.Name != "list-all-users" {
		t.Errorf("expected name 'list-all-users', got %q", spec.Name)
	}
}

// entryCtxKey is a context key for testing OnEntry receives context.
type entryCtxKey struct{}

// entryCtxInput implements Entryable and reads from context.
type entryCtxInput struct {
	Name string `json:"name"`
}

func (e *entryCtxInput) OnEntry(ctx context.Context) error {
	if v, ok := ctx.Value(entryCtxKey{}).(string); ok {
		e.Name = v + "-" + e.Name
	}
	return nil
}

// entryInput implements Entryable for testing.
type entryInput struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (e *entryInput) OnEntry(_ context.Context) error {
	e.Name = "modified-" + e.Name
	e.Count = e.Count * 10
	return nil
}

// entryInputError implements Entryable and always returns an error.
type entryInputError struct {
	Name string `json:"name"`
}

func (e *entryInputError) OnEntry(_ context.Context) error {
	return errors.New("entry hook failed")
}

// sendOutput implements Sendable for testing.
type sendOutput struct {
	Message string `json:"message"`
	Result  int    `json:"result"`
}

func (s *sendOutput) OnSend(_ context.Context) error {
	s.Message = s.Message + "-transformed"
	s.Result = s.Result * 2
	return nil
}

// sendOutputError implements Sendable and always returns an error.
type sendOutputError struct {
	Message string `json:"message"`
}

func (s *sendOutputError) OnSend(_ context.Context) error {
	return errors.New("send hook failed")
}

func TestHandler_OnEntry(t *testing.T) {
	handler := NewHandler[entryInput, testOutput](
		"test",
		"POST",
		"/test",
		func(req *Request[entryInput]) (testOutput, error) {
			return testOutput{
				Message: req.Body.Name,
				Result:  req.Body.Count,
			}, nil
		},
	)

	body, _ := json.Marshal(entryInput{Name: "original", Count: 3})
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output testOutput
	json.Unmarshal(w.Body.Bytes(), &output)

	if output.Message != "modified-original" {
		t.Errorf("expected 'modified-original', got %q", output.Message)
	}
	if output.Result != 30 {
		t.Errorf("expected 30, got %d", output.Result)
	}
}

func TestHandler_OnSend(t *testing.T) {
	handler := NewHandler[NoBody, sendOutput](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (sendOutput, error) {
			return sendOutput{Message: "hello", Result: 5}, nil
		},
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output sendOutput
	json.Unmarshal(w.Body.Bytes(), &output)

	if output.Message != "hello-transformed" {
		t.Errorf("expected 'hello-transformed', got %q", output.Message)
	}
	if output.Result != 10 {
		t.Errorf("expected 10, got %d", output.Result)
	}
}

func TestHandler_OnEntry_Error(t *testing.T) {
	handler := NewHandler[entryInputError, testOutput](
		"test",
		"POST",
		"/test",
		func(_ *Request[entryInputError]) (testOutput, error) {
			return testOutput{Message: "should not reach"}, nil
		},
	)

	body, _ := json.Marshal(entryInputError{Name: "test"})
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected error from entry hook")
	}
	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandler_OnSend_Error(t *testing.T) {
	handler := NewHandler[NoBody, sendOutputError](
		"test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (sendOutputError, error) {
			return sendOutputError{Message: "hello"}, nil
		},
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, err := handler.Process(context.Background(), req, w)

	if err == nil {
		t.Fatal("expected error from send hook")
	}
	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandler_OnEntry_ReceivesContext(t *testing.T) {
	handler := NewHandler[entryCtxInput, testOutput](
		"test",
		"POST",
		"/test",
		func(req *Request[entryCtxInput]) (testOutput, error) {
			return testOutput{Message: req.Body.Name}, nil
		},
	)

	body, _ := json.Marshal(entryCtxInput{Name: "original"})
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	ctx := context.WithValue(context.Background(), entryCtxKey{}, "from-context")
	_, err := handler.Process(ctx, req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output testOutput
	json.Unmarshal(w.Body.Bytes(), &output)

	if output.Message != "from-context-original" {
		t.Errorf("expected 'from-context-original', got %q", output.Message)
	}
}

func TestHTTPMethodShortcuts_Chaining(t *testing.T) {
	handler := GET[NoBody, testOutput]("/users", func(r *Request[NoBody]) (testOutput, error) {
		return testOutput{}, nil
	}).
		WithName("list-users").
		WithSummary("List all users").
		WithTags("users").
		WithAuthentication()

	spec := handler.Spec()
	if spec.Name != "list-users" {
		t.Errorf("expected name 'list-users', got %q", spec.Name)
	}
	if spec.Summary != "List all users" {
		t.Errorf("expected summary 'List all users', got %q", spec.Summary)
	}
	if !spec.RequiresAuth {
		t.Error("expected RequiresAuth to be true")
	}
}

// TestWriteError_AlwaysJSONContentType verifies that error responses carry a
// JSON content type even when the handler uses a non-JSON codec, since error
// bodies are always JSON-encoded.
func TestWriteError_AlwaysJSONContentType(t *testing.T) {
	handler := NewHandler[testInput, testOutput](
		"test-codec-error",
		"POST",
		"/test",
		func(_ *Request[testInput]) (testOutput, error) {
			return testOutput{}, ErrNotFound
		},
	).WithCodec(mockCodec{contentType: "application/x-custom"}).
		WithErrors(ErrNotFound)

	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"name":"x","value":1}`))
	w := httptest.NewRecorder()

	status, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
	if ct := w.Header().Get("Content-Type"); ct != ContentTypeJSON {
		t.Errorf("error response Content-Type = %q, want %q", ct, ContentTypeJSON)
	}
	var resp errorResponse
	if unmarshalErr := json.Unmarshal(w.Body.Bytes(), &resp); unmarshalErr != nil {
		t.Fatalf("error body is not JSON: %v", unmarshalErr)
	}
	if resp.Code != "NOT_FOUND" {
		t.Errorf("error code = %q, want NOT_FOUND", resp.Code)
	}
}

// TestNewHandler_ContractDefaults verifies the request/response contracts
// derived from the type parameters at construction.
func TestNewHandler_ContractDefaults(t *testing.T) {
	plain := NewHandler[testInput, testOutput]("plain", "POST", "/p",
		func(_ *Request[testInput]) (testOutput, error) { return testOutput{}, nil })
	spec := plain.Spec()
	if spec.Request.Kind != BodyEncoded {
		t.Errorf("Request.Kind = %q, want %q", spec.Request.Kind, BodyEncoded)
	}
	if spec.Request.MaxBytes != 10*1024*1024 {
		t.Errorf("Request.MaxBytes = %d, want 10MB", spec.Request.MaxBytes)
	}
	if spec.Response.Kind != BodyEncoded || spec.Response.Status != 200 {
		t.Errorf("Response = %+v, want encoded/200", spec.Response)
	}

	noBody := NewHandler[NoBody, testOutput]("nobody", "GET", "/n",
		func(_ *Request[NoBody]) (testOutput, error) { return testOutput{}, nil })
	if noBody.Spec().Request.Kind != BodyNone {
		t.Errorf("NoBody input: Request.Kind = %q, want %q", noBody.Spec().Request.Kind, BodyNone)
	}

	redirect := NewHandler[NoBody, Redirect]("redir", "GET", "/r",
		func(_ *Request[NoBody]) (Redirect, error) { return Redirect{URL: "/x"}, nil })
	rc := redirect.Spec().Response
	if rc.Kind != BodyNone || !rc.Redirect || rc.Status != DefaultRedirectStatus {
		t.Errorf("Redirect output: Response = %+v, want none/redirect/302", rc)
	}
}

// TestHandler_Process_RedirectValueStatusOverride verifies a nonzero
// Redirect.Status on the returned value overrides the declared status.
func TestHandler_Process_RedirectValueStatusOverride(t *testing.T) {
	handler := NewHandler[NoBody, Redirect]("redir", "GET", "/r",
		func(_ *Request[NoBody]) (Redirect, error) {
			return Redirect{URL: "/x", Status: http.StatusSeeOther}, nil
		})

	req := httptest.NewRequest("GET", "/r", nil)
	w := httptest.NewRecorder()
	status, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusSeeOther || w.Code != http.StatusSeeOther {
		t.Errorf("status = %d/%d, want 303", status, w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/x" {
		t.Errorf("Location = %q, want /x", loc)
	}
}
