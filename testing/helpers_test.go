package testing

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/zoobzio/rocco"
)

func TestResponseCapture(t *testing.T) {
	capture := NewResponseCapture()
	capture.WriteHeader(http.StatusCreated)
	capture.Write([]byte(`{"message":"test"}`))

	if capture.StatusCode() != http.StatusCreated {
		t.Errorf("expected status 201, got %d", capture.StatusCode())
	}

	if capture.BodyString() != `{"message":"test"}` {
		t.Errorf("unexpected body: %s", capture.BodyString())
	}

	var resp struct {
		Message string `json:"message"`
	}
	if err := capture.DecodeJSON(&resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if resp.Message != "test" {
		t.Errorf("expected message 'test', got %q", resp.Message)
	}
}

func TestResponseCapture_ContentType(t *testing.T) {
	capture := NewResponseCapture()
	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)

	if capture.ContentType() != "application/json" {
		t.Errorf("expected content-type 'application/json', got %q", capture.ContentType())
	}
}

func TestRequestBuilder(t *testing.T) {
	req := NewRequestBuilder("POST", "/users").
		WithHeader("Authorization", "Bearer token").
		WithHeader("X-Custom", "value").
		Build()

	if req.Method != "POST" {
		t.Errorf("expected method POST, got %s", req.Method)
	}
	if req.URL.Path != "/users" {
		t.Errorf("expected path /users, got %s", req.URL.Path)
	}
	if req.Header.Get("Authorization") != "Bearer token" {
		t.Errorf("expected Authorization header, got %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("X-Custom") != "value" {
		t.Errorf("expected X-Custom header, got %q", req.Header.Get("X-Custom"))
	}
}

func TestRequestBuilder_WithJSON(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}

	req := NewRequestBuilder("POST", "/users").
		WithJSON(input{Name: "test"}).
		Build()

	body := make([]byte, 100)
	n, _ := req.Body.Read(body)

	if string(body[:n]) != `{"name":"test"}` {
		t.Errorf("unexpected body: %s", string(body[:n]))
	}
}

func TestRequestBuilder_WithBody(t *testing.T) {
	req := NewRequestBuilder("POST", "/data").
		WithBody(bytes.NewReader([]byte("raw data"))).
		Build()

	body := make([]byte, 100)
	n, _ := req.Body.Read(body)

	if string(body[:n]) != "raw data" {
		t.Errorf("unexpected body: %s", string(body[:n]))
	}
}

func TestRequestBuilder_WithContext(t *testing.T) {
	type contextKey string
	key := contextKey("test")
	ctx := context.WithValue(context.Background(), key, "value")

	req := NewRequestBuilder("GET", "/test").
		WithContext(ctx).
		Build()

	if req.Context().Value(key) != "value" {
		t.Error("context value not preserved")
	}
}

func TestMockIdentity(t *testing.T) {
	identity := NewMockIdentity("user-123").
		WithTenantID("tenant-456").
		WithScopes("read", "write").
		WithRoles("admin", "user").
		WithStat("requests", 100)

	if identity.ID() != "user-123" {
		t.Errorf("expected ID 'user-123', got %q", identity.ID())
	}
	if identity.TenantID() != "tenant-456" {
		t.Errorf("expected TenantID 'tenant-456', got %q", identity.TenantID())
	}

	if !identity.HasScope("read") {
		t.Error("expected HasScope('read') to be true")
	}
	if !identity.HasScope("write") {
		t.Error("expected HasScope('write') to be true")
	}
	if identity.HasScope("delete") {
		t.Error("expected HasScope('delete') to be false")
	}

	if !identity.HasRole("admin") {
		t.Error("expected HasRole('admin') to be true")
	}
	if identity.HasRole("superuser") {
		t.Error("expected HasRole('superuser') to be false")
	}

	if identity.Stats()["requests"] != 100 {
		t.Errorf("expected requests stat 100, got %d", identity.Stats()["requests"])
	}
}

func TestMockIdentity_WithStats(t *testing.T) {
	stats := map[string]int{
		"requests":   100,
		"api_calls":  50,
		"rate_limit": 1000,
	}

	identity := NewMockIdentity("user").WithStats(stats)

	if identity.Stats()["requests"] != 100 {
		t.Errorf("expected requests 100, got %d", identity.Stats()["requests"])
	}
	if identity.Stats()["api_calls"] != 50 {
		t.Errorf("expected api_calls 50, got %d", identity.Stats()["api_calls"])
	}
}

func TestTestEngine(t *testing.T) {
	engine := TestEngine()
	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
}

func TestTestEngineWithAuth(t *testing.T) {
	identity := NewMockIdentity("test-user")
	engine := TestEngineWithAuth(func(_ context.Context, _ *http.Request) (rocco.Identity, error) {
		return identity, nil
	})
	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
}

type testOutput struct {
	Message string `json:"message"`
}

func TestServeRequest(t *testing.T) {
	engine := TestEngine()

	handler := rocco.NewHandler[rocco.NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(_ *rocco.Request[rocco.NoBody]) (testOutput, error) {
			return testOutput{Message: "hello"}, nil
		},
	)
	engine.WithHandlers(handler)

	capture := ServeRequest(engine, "GET", "/test", nil)

	if capture.StatusCode() != http.StatusOK {
		t.Errorf("expected status 200, got %d", capture.StatusCode())
	}

	var resp testOutput
	capture.DecodeJSON(&resp)
	if resp.Message != "hello" {
		t.Errorf("expected message 'hello', got %q", resp.Message)
	}
}

func TestServeRequestWithHeaders(t *testing.T) {
	engine := TestEngine()

	handler := rocco.NewHandler[rocco.NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(req *rocco.Request[rocco.NoBody]) (testOutput, error) {
			token := req.Header.Get("Authorization")
			return testOutput{Message: token}, nil
		},
	)
	engine.WithHandlers(handler)

	headers := map[string]string{"Authorization": "Bearer test-token"}
	capture := ServeRequestWithHeaders(engine, "GET", "/test", nil, headers)

	var resp testOutput
	capture.DecodeJSON(&resp)
	if resp.Message != "Bearer test-token" {
		t.Errorf("expected message with token, got %q", resp.Message)
	}
}

func TestResponseCapture_BodyBytes(t *testing.T) {
	capture := NewResponseCapture()
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"data":"test"}`))

	bodyBytes := capture.BodyBytes()
	if string(bodyBytes) != `{"data":"test"}` {
		t.Errorf("expected body bytes, got %s", string(bodyBytes))
	}
}

func TestMockIdentity_Scopes(t *testing.T) {
	identity := NewMockIdentity("user").WithScopes("read", "write", "delete")

	scopes := identity.Scopes()
	if len(scopes) != 3 {
		t.Fatalf("expected 3 scopes, got %d", len(scopes))
	}
	if scopes[0] != "read" || scopes[1] != "write" || scopes[2] != "delete" {
		t.Errorf("unexpected scopes: %v", scopes)
	}
}

func TestMockIdentity_Roles(t *testing.T) {
	identity := NewMockIdentity("user").WithRoles("admin", "moderator")

	roles := identity.Roles()
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
	if roles[0] != "admin" || roles[1] != "moderator" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestAssertStatus_Success(t *testing.T) {
	capture := NewResponseCapture()
	capture.WriteHeader(http.StatusCreated)

	// Should not panic or fail for matching status
	AssertStatus(t, capture, http.StatusCreated)
}

func TestAssertJSON_Success(t *testing.T) {
	capture := NewResponseCapture()
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"name":"test","count":42}`))

	expected := map[string]any{
		"name":  "test",
		"count": float64(42),
	}

	// Should not panic or fail for matching JSON
	AssertJSON(t, capture, expected)
}

func TestAssertErrorCode_Success(t *testing.T) {
	capture := NewResponseCapture()
	capture.WriteHeader(http.StatusNotFound)
	capture.Write([]byte(`{"code":"NOT_FOUND","message":"not found"}`))

	// Should not panic or fail for matching code
	AssertErrorCode(t, capture, "NOT_FOUND")
}

func TestAssertContentType_Success(t *testing.T) {
	capture := NewResponseCapture()
	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)

	// Should not panic or fail for matching type
	AssertContentType(t, capture, "application/json")
}

// Streaming helper tests

func TestStreamCapture(t *testing.T) {
	capture := NewStreamCapture()

	// Write SSE headers and data
	capture.Header().Set("Content-Type", "text/event-stream")
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte("event: update\ndata: {\"message\":\"hello\"}\n\n"))
	capture.Flush()

	if !capture.IsSSE() {
		t.Error("expected IsSSE to return true")
	}
	if capture.ContentType() != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", capture.ContentType())
	}
	if capture.FlushCount() != 1 {
		t.Errorf("expected 1 flush, got %d", capture.FlushCount())
	}
}

func TestStreamCapture_ParseEvents(t *testing.T) {
	capture := NewStreamCapture()
	capture.Header().Set("Content-Type", "text/event-stream")
	capture.WriteHeader(http.StatusOK)

	// Write multiple events
	capture.Write([]byte("data: {\"count\":1}\n\n"))
	capture.Write([]byte("event: custom\ndata: {\"count\":2}\n\n"))
	capture.Write([]byte("data: {\"count\":3}\n\n"))

	events := capture.ParseEvents()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// First event - data only
	if events[0].Event != "" {
		t.Errorf("expected empty event type, got %q", events[0].Event)
	}
	if events[0].Data != `{"count":1}` {
		t.Errorf("unexpected data: %q", events[0].Data)
	}

	// Second event - named event
	if events[1].Event != "custom" {
		t.Errorf("expected event type 'custom', got %q", events[1].Event)
	}

	// Third event - data only
	if events[2].Data != `{"count":3}` {
		t.Errorf("unexpected data: %q", events[2].Data)
	}
}

func TestStreamCapture_EventCount(t *testing.T) {
	capture := NewStreamCapture()
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte("data: event1\n\ndata: event2\n\ndata: event3\n\n"))

	count := capture.EventCount()
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

func TestParseSSEEvents(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected []SSEEvent
	}{
		{
			name:     "empty body",
			body:     "",
			expected: []SSEEvent{},
		},
		{
			name: "single data event",
			body: "data: hello\n\n",
			expected: []SSEEvent{
				{Data: "hello"},
			},
		},
		{
			name: "named event",
			body: "event: message\ndata: test\n\n",
			expected: []SSEEvent{
				{Event: "message", Data: "test"},
			},
		},
		{
			name: "event with id",
			body: "id: 123\ndata: test\n\n",
			expected: []SSEEvent{
				{ID: "123", Data: "test"},
			},
		},
		{
			name: "multiple events",
			body: "data: first\n\ndata: second\n\n",
			expected: []SSEEvent{
				{Data: "first"},
				{Data: "second"},
			},
		},
		{
			name: "event with comment",
			body: ": this is a comment\ndata: hello\n\n",
			expected: []SSEEvent{
				{Data: "hello"},
			},
		},
		{
			name: "no trailing newline",
			body: "data: final",
			expected: []SSEEvent{
				{Data: "final"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := ParseSSEEvents(tt.body)
			if len(events) != len(tt.expected) {
				t.Fatalf("expected %d events, got %d", len(tt.expected), len(events))
			}
			for i, expected := range tt.expected {
				if events[i].Event != expected.Event {
					t.Errorf("event[%d].Event = %q, want %q", i, events[i].Event, expected.Event)
				}
				if events[i].Data != expected.Data {
					t.Errorf("event[%d].Data = %q, want %q", i, events[i].Data, expected.Data)
				}
				if events[i].ID != expected.ID {
					t.Errorf("event[%d].ID = %q, want %q", i, events[i].ID, expected.ID)
				}
			}
		})
	}
}

func TestSSEEvent_DecodeJSON(t *testing.T) {
	event := SSEEvent{Data: `{"name":"test","value":42}`}

	var result struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	if err := event.DecodeJSON(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got %q", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

type streamOutput struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func TestServeStream(t *testing.T) {
	engine := TestEngine()

	handler := rocco.NewStreamHandler[rocco.NoBody, streamOutput](
		"test-stream",
		"GET",
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamOutput]) error {
			stream.Send(streamOutput{Message: "hello", Count: 1})
			stream.Send(streamOutput{Message: "world", Count: 2})
			return nil
		},
	)
	engine.WithHandlers(handler)

	capture := ServeStream(engine, "GET", "/events", nil)

	if !capture.IsSSE() {
		t.Error("expected SSE response")
	}

	events := capture.ParseEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestServeStreamWithContext(t *testing.T) {
	engine := TestEngine()

	handler := rocco.NewStreamHandler[rocco.NoBody, streamOutput](
		"test-stream",
		"GET",
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamOutput]) error {
			return stream.Send(streamOutput{Message: "test", Count: 1})
		},
	)
	engine.WithHandlers(handler)

	ctx := context.Background()
	capture := ServeStreamWithContext(ctx, engine, "GET", "/events", nil)

	if capture.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", capture.Code)
	}
}

func TestServeStreamWithHeaders(t *testing.T) {
	engine := TestEngine()

	var receivedToken string
	handler := rocco.NewStreamHandler[rocco.NoBody, streamOutput](
		"test-stream",
		"GET",
		"/events",
		func(req *rocco.Request[rocco.NoBody], stream rocco.Stream[streamOutput]) error {
			receivedToken = req.Header.Get("Authorization")
			return stream.Send(streamOutput{Message: "ok", Count: 1})
		},
	)
	engine.WithHandlers(handler)

	headers := map[string]string{"Authorization": "Bearer stream-token"}
	capture := ServeStreamWithHeaders(engine, "GET", "/events", nil, headers)

	if capture.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", capture.Code)
	}
	if receivedToken != "Bearer stream-token" {
		t.Errorf("expected token 'Bearer stream-token', got %q", receivedToken)
	}
}

func TestAssertSSE(t *testing.T) {
	capture := NewStreamCapture()
	capture.Header().Set("Content-Type", "text/event-stream")
	capture.WriteHeader(http.StatusOK)

	// Should not fail
	AssertSSE(t, capture)
}

func TestAssertEventCount(t *testing.T) {
	capture := NewStreamCapture()
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte("data: one\n\ndata: two\n\n"))

	// Should not fail
	AssertEventCount(t, capture, 2)
}

func TestDecodeJSON(t *testing.T) {
	capture := NewResponseCapture()
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"name":"test","count":42}`))

	var result struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	if err := capture.DecodeJSON(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if result.Name != "test" {
		t.Errorf("expected name 'test', got %q", result.Name)
	}
	if result.Count != 42 {
		t.Errorf("expected count 42, got %d", result.Count)
	}
}
