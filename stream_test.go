package rocco

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type streamEvent struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type streamInput struct {
	Topic string `json:"topic" validate:"required"`
}

func TestNewStreamHandler(t *testing.T) {
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], _ Stream[streamEvent]) error {
			return nil
		},
	)

	spec := handler.Spec()
	if spec.Name != "test-stream" {
		t.Errorf("expected name 'test-stream', got %q", spec.Name)
	}
	if spec.Method != "GET" {
		t.Errorf("expected method 'GET', got %q", spec.Method)
	}
	if spec.Path != "/events" {
		t.Errorf("expected path '/events', got %q", spec.Path)
	}
	if !spec.IsStream {
		t.Error("expected IsStream to be true")
	}
	if spec.SuccessStatus != 200 {
		t.Errorf("expected default success status 200, got %d", spec.SuccessStatus)
	}
	if handler.InputMeta.TypeName != "NoBody" {
		t.Errorf("expected input type 'NoBody', got %q", handler.InputMeta.TypeName)
	}
	if handler.OutputMeta.TypeName != "streamEvent" {
		t.Errorf("expected output type 'streamEvent', got %q", handler.OutputMeta.TypeName)
	}
}

func TestStreamHandler_WithBuilderMethods(t *testing.T) {
	handler := NewStreamHandler[streamInput, streamEvent](
		"test-stream",
		"GET",
		"/events/{topic}",
		func(_ *Request[streamInput], _ Stream[streamEvent]) error {
			return nil
		},
	).
		WithSummary("Event stream").
		WithDescription("Streams events in real-time").
		WithTags("streaming", "events").
		WithPathParams("topic").
		WithQueryParams("filter").
		WithErrors(ErrBadRequest, ErrUnauthorized).
		WithAuthentication().
		WithScopes("events:read").
		WithRoles("subscriber")

	spec := handler.Spec()
	if spec.Summary != "Event stream" {
		t.Errorf("expected summary 'Event stream', got %q", spec.Summary)
	}
	if spec.Description != "Streams events in real-time" {
		t.Errorf("expected description, got %q", spec.Description)
	}
	if len(spec.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(spec.Tags))
	}
	if !spec.IsStream {
		t.Error("expected IsStream to be true")
	}
	if len(spec.PathParams) != 1 {
		t.Errorf("expected 1 path param, got %d", len(spec.PathParams))
	}
	if len(spec.QueryParams) != 1 {
		t.Errorf("expected 1 query param, got %d", len(spec.QueryParams))
	}
	if len(spec.ErrorCodes) != 2 {
		t.Errorf("expected 2 error codes, got %d", len(spec.ErrorCodes))
	}
	if !spec.RequiresAuth {
		t.Error("expected RequiresAuth to be true")
	}
	if len(spec.ScopeGroups) != 1 {
		t.Errorf("expected 1 scope group, got %d", len(spec.ScopeGroups))
	}
	if len(spec.RoleGroups) != 1 {
		t.Errorf("expected 1 role group, got %d", len(spec.RoleGroups))
	}
}

// flushRecorder wraps httptest.ResponseRecorder to implement http.Flusher
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

func (f *flushRecorder) Flush() {
	f.flushed++
}

func TestStreamHandler_Process_SendEvents(t *testing.T) {
	eventsSent := 0
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], stream Stream[streamEvent]) error {
			for i := 0; i < 3; i++ {
				if err := stream.Send(streamEvent{
					Message: "test",
					Count:   i,
				}); err != nil {
					return err
				}
				eventsSent++
			}
			return nil
		},
	)

	req := httptest.NewRequest("GET", "/events", nil)
	w := newFlushRecorder()

	status, err := handler.Process(context.Background(), req, w)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 200 {
		t.Errorf("expected status 200, got %d", status)
	}
	if eventsSent != 3 {
		t.Errorf("expected 3 events sent, got %d", eventsSent)
	}

	// Check headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %q", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("expected Connection 'keep-alive', got %q", conn)
	}

	// Parse SSE events from body
	body := w.Body.String()
	events := parseSSEEvents(body)
	if len(events) != 3 {
		t.Errorf("expected 3 events in body, got %d", len(events))
	}

	// Verify flush was called for each event
	if w.flushed != 3 {
		t.Errorf("expected 3 flushes, got %d", w.flushed)
	}
}

func TestStreamHandler_Process_SendNamedEvents(t *testing.T) {
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], stream Stream[streamEvent]) error {
			return stream.SendEvent("update", streamEvent{
				Message: "named event",
				Count:   42,
			})
		},
	)

	req := httptest.NewRequest("GET", "/events", nil)
	w := newFlushRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: update") {
		t.Error("expected body to contain 'event: update'")
	}
	if !strings.Contains(body, `"message":"named event"`) {
		t.Error("expected body to contain event data")
	}
}

func TestStreamHandler_Process_SendComment(t *testing.T) {
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], stream Stream[streamEvent]) error {
			return stream.SendComment("keep-alive")
		},
	)

	req := httptest.NewRequest("GET", "/events", nil)
	w := newFlushRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, ": keep-alive") {
		t.Errorf("expected body to contain ': keep-alive', got %q", body)
	}
}

func TestStreamHandler_Process_ClientDisconnect(t *testing.T) {
	started := make(chan struct{})
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(req *Request[NoBody], stream Stream[streamEvent]) error {
			close(started)
			// Wait for client disconnect
			<-stream.Done()
			return nil
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/events", nil).WithContext(ctx)
	w := newFlushRecorder()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler.Process(ctx, req, w)
	}()

	// Wait for handler to start
	<-started
	// Cancel context to simulate client disconnect
	cancel()
	wg.Wait()

	// Handler should complete without error on client disconnect
	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestStreamHandler_Process_WithPathParams(t *testing.T) {
	var receivedTopic string
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events/{topic}",
		func(req *Request[NoBody], stream Stream[streamEvent]) error {
			receivedTopic = req.Params.Path["topic"]
			return stream.Send(streamEvent{Message: receivedTopic, Count: 1})
		},
	).WithPathParams("topic")

	// Create request with path value set
	req := httptest.NewRequest("GET", "/events/news", nil)
	req.SetPathValue("topic", "news")
	w := newFlushRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedTopic != "news" {
		t.Errorf("expected topic 'news', got %q", receivedTopic)
	}
}

func TestStreamHandler_Process_WithInputBody(t *testing.T) {
	var receivedInput streamInput
	handler := NewStreamHandler[streamInput, streamEvent](
		"test-stream",
		"POST",
		"/events",
		func(req *Request[streamInput], stream Stream[streamEvent]) error {
			receivedInput = req.Body
			return stream.Send(streamEvent{Message: req.Body.Topic, Count: 1})
		},
	)

	input := streamInput{Topic: "updates"}
	body, _ := json.Marshal(input)

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
	w := newFlushRecorder()

	_, err := handler.Process(context.Background(), req, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedInput.Topic != "updates" {
		t.Errorf("expected topic 'updates', got %q", receivedInput.Topic)
	}
}

func TestStreamHandler_Process_ValidationError(t *testing.T) {
	handler := NewStreamHandler[streamInput, streamEvent](
		"test-stream",
		"POST",
		"/events",
		func(_ *Request[streamInput], _ Stream[streamEvent]) error {
			t.Error("handler should not be called on validation error")
			return nil
		},
	)

	// Empty topic should fail validation (required)
	input := streamInput{Topic: ""}
	body, _ := json.Marshal(input)

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
	w := newFlushRecorder()

	status, err := handler.Process(context.Background(), req, w)
	if err == nil {
		t.Error("expected validation error")
	}
	if status != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", status)
	}
}

func TestStreamHandler_Process_InvalidJSON(t *testing.T) {
	handler := NewStreamHandler[streamInput, streamEvent](
		"test-stream",
		"POST",
		"/events",
		func(_ *Request[streamInput], _ Stream[streamEvent]) error {
			t.Error("handler should not be called on parse error")
			return nil
		},
	)

	req := httptest.NewRequest("POST", "/events", strings.NewReader("{invalid}"))
	w := newFlushRecorder()

	status, err := handler.Process(context.Background(), req, w)
	if err == nil {
		t.Error("expected parse error")
	}
	if status != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", status)
	}
}

func TestStreamHandler_Process_MissingPathParam(t *testing.T) {
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events/{topic}",
		func(_ *Request[NoBody], _ Stream[streamEvent]) error {
			t.Error("handler should not be called on missing path param")
			return nil
		},
	).WithPathParams("topic")

	req := httptest.NewRequest("GET", "/events/", nil)
	w := newFlushRecorder()

	// No chi context - missing path params
	status, err := handler.Process(context.Background(), req, w)
	if err == nil {
		t.Error("expected error for missing path param")
	}
	if status != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", status)
	}
}

func TestStreamHandler_Middleware(t *testing.T) {
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], _ Stream[streamEvent]) error {
			return nil
		},
	).WithMiddleware(mw)

	middleware := handler.Middleware()
	if len(middleware) != 1 {
		t.Errorf("expected 1 middleware, got %d", len(middleware))
	}
}

func TestStreamHandler_ErrorDefs(t *testing.T) {
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], _ Stream[streamEvent]) error {
			return nil
		},
	).WithErrors(ErrBadRequest, ErrNotFound, ErrUnauthorized)

	defs := handler.ErrorDefs()
	if len(defs) != 3 {
		t.Errorf("expected 3 error defs, got %d", len(defs))
	}
}

func TestStreamHandler_Close(t *testing.T) {
	handler := NewStreamHandler[NoBody, streamEvent](
		"test-stream",
		"GET",
		"/events",
		func(_ *Request[NoBody], _ Stream[streamEvent]) error {
			return nil
		},
	)

	if err := handler.Close(); err != nil {
		t.Errorf("expected Close to return nil, got %v", err)
	}
}

func TestStream_SendAfterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	stream := &sseStream[streamEvent]{
		w:       newFlushRecorder(),
		flusher: newFlushRecorder(),
		done:    ctx.Done(),
	}

	err := stream.Send(streamEvent{Message: "test", Count: 1})
	if err == nil {
		t.Error("expected error when sending after close")
	}
	if !strings.Contains(err.Error(), "client disconnected") {
		t.Errorf("expected 'client disconnected' error, got %q", err.Error())
	}
}

func TestStream_Done(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	stream := &sseStream[streamEvent]{
		w:       newFlushRecorder(),
		flusher: newFlushRecorder(),
		done:    ctx.Done(),
	}

	// Channel should not be closed yet
	select {
	case <-stream.Done():
		t.Error("Done() should not be closed yet")
	default:
		// Expected
	}

	cancel()

	// Channel should be closed now
	select {
	case <-stream.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("Done() should be closed after cancel")
	}
}

// parseSSEEvents parses SSE event data from a response body
func parseSSEEvents(body string) []map[string]any {
	var events []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(body))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				events = append(events, event)
			}
		}
	}

	return events
}
