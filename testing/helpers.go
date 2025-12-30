// Package testing provides test utilities for rocco.
package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zoobzio/rocco"
)

// ResponseCapture wraps httptest.ResponseRecorder with convenient access methods.
type ResponseCapture struct {
	*httptest.ResponseRecorder
}

// NewResponseCapture creates a new ResponseCapture.
func NewResponseCapture() *ResponseCapture {
	return &ResponseCapture{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

// StatusCode returns the recorded status code.
func (r *ResponseCapture) StatusCode() int {
	return r.Code
}

// BodyBytes returns the response body as bytes.
func (r *ResponseCapture) BodyBytes() []byte {
	return r.Body.Bytes()
}

// BodyString returns the response body as a string.
func (r *ResponseCapture) BodyString() string {
	return r.Body.String()
}

// DecodeJSON decodes the response body into the provided value.
func (r *ResponseCapture) DecodeJSON(v any) error {
	return json.Unmarshal(r.Body.Bytes(), v)
}

// ContentType returns the Content-Type header value.
func (r *ResponseCapture) ContentType() string {
	return r.Header().Get("Content-Type")
}

// RequestBuilder provides a fluent interface for building test requests.
type RequestBuilder struct {
	method  string
	path    string
	body    io.Reader
	headers map[string]string
	ctx     context.Context
}

// NewRequestBuilder creates a new RequestBuilder with the given method and path.
func NewRequestBuilder(method, path string) *RequestBuilder {
	return &RequestBuilder{
		method:  method,
		path:    path,
		headers: make(map[string]string),
		ctx:     context.Background(),
	}
}

// WithBody sets the request body from a reader.
func (b *RequestBuilder) WithBody(body io.Reader) *RequestBuilder {
	b.body = body
	return b
}

// WithJSON sets the request body as JSON-encoded data.
func (b *RequestBuilder) WithJSON(v any) *RequestBuilder {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("rtesting: failed to marshal JSON: %v", err))
	}
	b.body = bytes.NewReader(data)
	return b
}

// WithHeader adds a header to the request.
func (b *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	b.headers[key] = value
	return b
}

// WithContext sets the request context.
func (b *RequestBuilder) WithContext(ctx context.Context) *RequestBuilder {
	b.ctx = ctx
	return b
}

// Build creates the http.Request.
func (b *RequestBuilder) Build() *http.Request {
	req := httptest.NewRequest(b.method, b.path, b.body)
	req = req.WithContext(b.ctx)
	for key, value := range b.headers {
		req.Header.Set(key, value)
	}
	return req
}

// TestEngine creates a pre-configured engine for testing.
func TestEngine() *rocco.Engine {
	return rocco.NewEngine("localhost", 0, nil)
}

// TestEngineWithAuth creates an engine with a mock identity extractor.
func TestEngineWithAuth(extractor func(context.Context, *http.Request) (rocco.Identity, error)) *rocco.Engine {
	return rocco.NewEngine("localhost", 0, extractor)
}

// ServeRequest is a convenience function that executes a request against an engine.
func ServeRequest(engine *rocco.Engine, method, path string, body any) *ResponseCapture {
	builder := NewRequestBuilder(method, path)
	if body != nil {
		builder.WithJSON(body)
	}
	req := builder.Build()

	capture := NewResponseCapture()
	engine.Router().ServeHTTP(capture, req)
	return capture
}

// ServeRequestWithHeaders executes a request with custom headers.
func ServeRequestWithHeaders(engine *rocco.Engine, method, path string, body any, headers map[string]string) *ResponseCapture {
	builder := NewRequestBuilder(method, path)
	if body != nil {
		builder.WithJSON(body)
	}
	for key, value := range headers {
		builder.WithHeader(key, value)
	}
	req := builder.Build()

	capture := NewResponseCapture()
	engine.Router().ServeHTTP(capture, req)
	return capture
}

// MockIdentity implements rocco.Identity for testing.
type MockIdentity struct {
	id       string
	tenantID string
	scopes   []string
	roles    []string
	stats    map[string]int
}

// NewMockIdentity creates a new MockIdentity with the given ID.
func NewMockIdentity(id string) *MockIdentity {
	return &MockIdentity{
		id:     id,
		scopes: make([]string, 0),
		roles:  make([]string, 0),
		stats:  make(map[string]int),
	}
}

// ID returns the identity ID.
func (m *MockIdentity) ID() string { return m.id }

// TenantID returns the tenant ID.
func (m *MockIdentity) TenantID() string { return m.tenantID }

// Scopes returns the identity scopes.
func (m *MockIdentity) Scopes() []string { return m.scopes }

// Roles returns the identity roles.
func (m *MockIdentity) Roles() []string { return m.roles }

// Stats returns the identity stats.
func (m *MockIdentity) Stats() map[string]int { return m.stats }

// HasScope checks if the identity has the given scope.
func (m *MockIdentity) HasScope(scope string) bool {
	for _, s := range m.scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// HasRole checks if the identity has the given role.
func (m *MockIdentity) HasRole(role string) bool {
	for _, r := range m.roles {
		if r == role {
			return true
		}
	}
	return false
}

// WithTenantID sets the tenant ID.
func (m *MockIdentity) WithTenantID(tenantID string) *MockIdentity {
	m.tenantID = tenantID
	return m
}

// WithScopes sets the scopes.
func (m *MockIdentity) WithScopes(scopes ...string) *MockIdentity {
	m.scopes = scopes
	return m
}

// WithRoles sets the roles.
func (m *MockIdentity) WithRoles(roles ...string) *MockIdentity {
	m.roles = roles
	return m
}

// WithStats sets the stats.
func (m *MockIdentity) WithStats(stats map[string]int) *MockIdentity {
	m.stats = stats
	return m
}

// WithStat sets a single stat value.
func (m *MockIdentity) WithStat(key string, value int) *MockIdentity {
	m.stats[key] = value
	return m
}

// AssertStatus asserts the response has the expected status code.
func AssertStatus(t testing.TB, capture *ResponseCapture, expected int) {
	t.Helper()
	if capture.StatusCode() != expected {
		t.Errorf("expected status %d, got %d (body: %s)", expected, capture.StatusCode(), capture.BodyString())
	}
}

// AssertJSON asserts the response body matches the expected value when decoded as JSON.
func AssertJSON(t testing.TB, capture *ResponseCapture, expected any) {
	t.Helper()
	expectedBytes, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("failed to marshal expected value: %v", err)
	}
	actualBytes := capture.BodyBytes()

	var expectedMap, actualMap any
	err = json.Unmarshal(expectedBytes, &expectedMap)
	if err != nil {
		t.Fatalf("failed to unmarshal expected JSON: %v", err)
	}
	err = json.Unmarshal(actualBytes, &actualMap)
	if err != nil {
		t.Fatalf("failed to unmarshal actual JSON: %v", err)
	}

	expectedNorm, err := json.Marshal(expectedMap)
	if err != nil {
		t.Fatalf("failed to normalize expected JSON: %v", err)
	}
	actualNorm, err := json.Marshal(actualMap)
	if err != nil {
		t.Fatalf("failed to normalize actual JSON: %v", err)
	}

	if !bytes.Equal(expectedNorm, actualNorm) {
		t.Errorf("JSON mismatch:\nexpected: %s\nactual:   %s", expectedNorm, actualNorm)
	}
}

// AssertErrorCode asserts the response is an error with the given code.
func AssertErrorCode(t testing.TB, capture *ResponseCapture, expectedCode string) {
	t.Helper()
	var resp struct {
		Code string `json:"code"`
	}
	if err := capture.DecodeJSON(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Code != expectedCode {
		t.Errorf("expected error code %q, got %q", expectedCode, resp.Code)
	}
}

// AssertContentType asserts the response has the expected Content-Type.
func AssertContentType(t testing.TB, capture *ResponseCapture, expected string) {
	t.Helper()
	if capture.ContentType() != expected {
		t.Errorf("expected Content-Type %q, got %q", expected, capture.ContentType())
	}
}

// SSE Test Helpers

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	Event string // Event type (empty for data-only events)
	Data  string // Raw data string
	ID    string // Event ID (if present)
}

// DecodeJSON decodes the event data into the provided value.
func (e *SSEEvent) DecodeJSON(v any) error {
	return json.Unmarshal([]byte(e.Data), v)
}

// StreamCapture wraps a response recorder with SSE-specific methods.
type StreamCapture struct {
	*httptest.ResponseRecorder
	flushed int
}

// NewStreamCapture creates a new StreamCapture.
func NewStreamCapture() *StreamCapture {
	return &StreamCapture{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

// Flush implements http.Flusher.
func (s *StreamCapture) Flush() {
	s.flushed++
}

// FlushCount returns the number of times Flush was called.
func (s *StreamCapture) FlushCount() int {
	return s.flushed
}

// ContentType returns the Content-Type header value.
func (s *StreamCapture) ContentType() string {
	return s.Header().Get("Content-Type")
}

// IsSSE returns true if the response has SSE content type.
func (s *StreamCapture) IsSSE() bool {
	return s.ContentType() == "text/event-stream"
}

// ParseEvents parses all SSE events from the response body.
func (s *StreamCapture) ParseEvents() []SSEEvent {
	return ParseSSEEvents(s.Body.String())
}

// EventCount returns the number of data events in the response.
func (s *StreamCapture) EventCount() int {
	return len(s.ParseEvents())
}

// ParseSSEEvents parses SSE events from a string.
func ParseSSEEvents(body string) []SSEEvent {
	var events []SSEEvent
	var currentEvent SSEEvent

	lines := splitLines(body)
	for _, line := range lines {
		switch {
		case line == "":
			// Empty line marks end of event
			if currentEvent.Data != "" {
				events = append(events, currentEvent)
				currentEvent = SSEEvent{}
			}
		case len(line) > 6 && line[:6] == "event:":
			currentEvent.Event = trimPrefix(line, "event:")
		case len(line) > 5 && line[:5] == "data:":
			currentEvent.Data = trimPrefix(line, "data:")
		case len(line) > 3 && line[:3] == "id:":
			currentEvent.ID = trimPrefix(line, "id:")
		case line[0] == ':':
			// Comment, ignore
		}
	}

	// Handle final event if no trailing newline
	if currentEvent.Data != "" {
		events = append(events, currentEvent)
	}

	return events
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	var current []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, string(current))
			current = nil
		} else if s[i] != '\r' {
			current = append(current, s[i])
		}
	}
	if len(current) > 0 {
		lines = append(lines, string(current))
	}
	return lines
}

// trimPrefix trims the prefix and leading/trailing whitespace.
func trimPrefix(s, prefix string) string {
	s = s[len(prefix):]
	// Trim leading space
	for s != "" && s[0] == ' ' {
		s = s[1:]
	}
	// Trim trailing space
	for s != "" && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// ServeStream executes a streaming request and returns a StreamCapture.
func ServeStream(engine *rocco.Engine, method, path string, body any) *StreamCapture {
	builder := NewRequestBuilder(method, path)
	if body != nil {
		builder.WithJSON(body)
	}
	req := builder.Build()

	capture := NewStreamCapture()
	engine.Router().ServeHTTP(capture, req)
	return capture
}

// ServeStreamWithContext executes a streaming request with a custom context.
func ServeStreamWithContext(ctx context.Context, engine *rocco.Engine, method, path string, body any) *StreamCapture {
	builder := NewRequestBuilder(method, path).WithContext(ctx)
	if body != nil {
		builder.WithJSON(body)
	}
	req := builder.Build()

	capture := NewStreamCapture()
	engine.Router().ServeHTTP(capture, req)
	return capture
}

// ServeStreamWithHeaders executes a streaming request with custom headers.
func ServeStreamWithHeaders(engine *rocco.Engine, method, path string, body any, headers map[string]string) *StreamCapture {
	builder := NewRequestBuilder(method, path)
	if body != nil {
		builder.WithJSON(body)
	}
	for key, value := range headers {
		builder.WithHeader(key, value)
	}
	req := builder.Build()

	capture := NewStreamCapture()
	engine.Router().ServeHTTP(capture, req)
	return capture
}

// AssertSSE asserts the response is a valid SSE stream.
func AssertSSE(t testing.TB, capture *StreamCapture) {
	t.Helper()
	if !capture.IsSSE() {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", capture.ContentType())
	}
	if capture.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", capture.Code)
	}
}

// AssertEventCount asserts the stream contains the expected number of events.
func AssertEventCount(t testing.TB, capture *StreamCapture, expected int) {
	t.Helper()
	actual := capture.EventCount()
	if actual != expected {
		t.Errorf("expected %d events, got %d", expected, actual)
	}
}
