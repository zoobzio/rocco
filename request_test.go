package rocco

import (
	"context"
	"net/http/httptest"
	"testing"
	"unsafe"
)

func TestRequest_EmbeddedContext(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	httpReq := httptest.NewRequest("GET", "/test", nil)

	req := &Request[NoBody]{
		Context: ctx,
		Request: httpReq,
		Params:  &Params{},
		Body:    NoBody{},
	}

	// Should be able to access context methods directly
	if val := req.Value(contextKey("key")); val != "value" {
		t.Errorf("expected context value 'value', got %v", val)
	}
}

func TestRequest_EmbeddedHTTPRequest(t *testing.T) {
	httpReq := httptest.NewRequest("POST", "/api/test", nil)
	httpReq.Header.Set("X-Custom", "test-value")

	req := &Request[NoBody]{
		Context: context.Background(),
		Request: httpReq,
		Params:  &Params{},
		Body:    NoBody{},
	}

	// Should be able to access http.Request fields directly
	if req.Method != "POST" {
		t.Errorf("expected method 'POST', got %q", req.Method)
	}
	if req.URL.Path != "/api/test" {
		t.Errorf("expected path '/api/test', got %q", req.URL.Path)
	}
	if req.Header.Get("X-Custom") != "test-value" {
		t.Errorf("expected header 'test-value', got %q", req.Header.Get("X-Custom"))
	}
}

func TestRequest_WithTypedBody(t *testing.T) {
	type TestBody struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	httpReq := httptest.NewRequest("POST", "/test", nil)
	body := TestBody{Name: "test", Count: 42}

	req := &Request[TestBody]{
		Context: context.Background(),
		Request: httpReq,
		Params:  &Params{},
		Body:    body,
	}

	if req.Body.Name != "test" {
		t.Errorf("expected body name 'test', got %q", req.Body.Name)
	}
	if req.Body.Count != 42 {
		t.Errorf("expected body count 42, got %d", req.Body.Count)
	}
}

func TestParams(t *testing.T) {
	params := &Params{
		Path: map[string]string{
			"id":   "123",
			"slug": "test-slug",
		},
		Query: map[string]string{
			"page":  "1",
			"limit": "10",
		},
	}

	// Test path params
	if params.Path["id"] != "123" {
		t.Errorf("expected path param 'id' = '123', got %q", params.Path["id"])
	}
	if params.Path["slug"] != "test-slug" {
		t.Errorf("expected path param 'slug' = 'test-slug', got %q", params.Path["slug"])
	}

	// Test query params
	if params.Query["page"] != "1" {
		t.Errorf("expected query param 'page' = '1', got %q", params.Query["page"])
	}
	if params.Query["limit"] != "10" {
		t.Errorf("expected query param 'limit' = '10', got %q", params.Query["limit"])
	}
}

func TestNoBody_IsZeroSized(t *testing.T) {
	var nb NoBody

	// NoBody should be zero-sized
	if size := int(unsafe.Sizeof(nb)); size != 0 {
		t.Errorf("expected NoBody to be zero-sized, got size %d", size)
	}
}
