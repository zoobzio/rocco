package integration

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/rocco"
	rtesting "github.com/zoobzio/rocco/testing"
)

// Test types for streaming.
type streamEvent struct {
	Message string `json:"message"`
	Seq     int    `json:"seq"`
}

type streamInput struct {
	Topic string `json:"topic" validate:"required"`
}

func TestStreamHandler_FullLifecycle(t *testing.T) {
	engine := rtesting.TestEngine()

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"lifecycle-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			// Send 5 events
			for i := 0; i < 5; i++ {
				if err := stream.Send(streamEvent{
					Message: "event",
					Seq:     i,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	).WithSummary("Full lifecycle test")

	engine.WithHandlers(handler)

	capture := rtesting.ServeStream(engine, "GET", "/events", nil)

	rtesting.AssertSSE(t, capture)
	rtesting.AssertEventCount(t, capture, 5)

	events := capture.ParseEvents()
	for i, event := range events {
		var e streamEvent
		if err := event.DecodeJSON(&e); err != nil {
			t.Fatalf("failed to decode event %d: %v", i, err)
		}
		if e.Seq != i {
			t.Errorf("event %d: expected seq %d, got %d", i, i, e.Seq)
		}
	}
}

func TestStreamHandler_NamedEvents(t *testing.T) {
	engine := rtesting.TestEngine()

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"named-events",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			stream.SendEvent("start", streamEvent{Message: "starting", Seq: 0})
			stream.SendEvent("update", streamEvent{Message: "processing", Seq: 1})
			stream.SendEvent("complete", streamEvent{Message: "done", Seq: 2})
			return nil
		},
	)

	engine.WithHandlers(handler)

	capture := rtesting.ServeStream(engine, "GET", "/events", nil)

	rtesting.AssertSSE(t, capture)

	events := capture.ParseEvents()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	expectedTypes := []string{"start", "update", "complete"}
	for i, event := range events {
		if event.Event != expectedTypes[i] {
			t.Errorf("event %d: expected type %q, got %q", i, expectedTypes[i], event.Event)
		}
	}
}

func TestStreamHandler_WithInputBody(t *testing.T) {
	engine := rtesting.TestEngine()

	handler := rocco.NewStreamHandler[streamInput, streamEvent](
		"input-stream",
		http.MethodPost,
		"/events",
		func(req *rocco.Request[streamInput], stream rocco.Stream[streamEvent]) error {
			// Echo back the topic in events
			for i := 0; i < 3; i++ {
				if err := stream.Send(streamEvent{
					Message: req.Body.Topic,
					Seq:     i,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	engine.WithHandlers(handler)

	capture := rtesting.ServeStream(engine, "POST", "/events", streamInput{Topic: "news"})

	rtesting.AssertSSE(t, capture)

	events := capture.ParseEvents()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	var first streamEvent
	events[0].DecodeJSON(&first)
	if first.Message != "news" {
		t.Errorf("expected message 'news', got %q", first.Message)
	}
}

func TestStreamHandler_ConcurrentClients(t *testing.T) {
	engine := rtesting.TestEngine()

	var connectionCount atomic.Int32

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"concurrent-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			connectionCount.Add(1)
			defer connectionCount.Add(-1)

			for i := 0; i < 3; i++ {
				if err := stream.Send(streamEvent{
					Message: "concurrent",
					Seq:     i,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	engine.WithHandlers(handler)

	const numClients = 10
	var wg sync.WaitGroup
	results := make([]*rtesting.StreamCapture, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = rtesting.ServeStream(engine, "GET", "/events", nil)
		}(i)
	}

	wg.Wait()

	// Verify all clients received events
	for i, capture := range results {
		if capture.Code != http.StatusOK {
			t.Errorf("client %d: expected status 200, got %d", i, capture.Code)
		}
		events := capture.ParseEvents()
		if len(events) != 3 {
			t.Errorf("client %d: expected 3 events, got %d", i, len(events))
		}
	}
}

func TestStreamHandler_ClientDisconnect(t *testing.T) {
	engine := rtesting.TestEngine()

	disconnected := make(chan struct{})
	started := make(chan struct{})

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"disconnect-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			close(started)
			<-stream.Done()
			close(disconnected)
			return nil
		},
	)

	engine.WithHandlers(handler)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rtesting.ServeStreamWithContext(ctx, engine, "GET", "/events", nil)
	}()

	// Wait for handler to start
	<-started

	// Simulate client disconnect
	cancel()

	// Wait for handler to detect disconnect
	select {
	case <-disconnected:
		// Success
	case <-time.After(time.Second):
		t.Fatal("handler did not detect client disconnect")
	}

	wg.Wait()
}

func TestStreamHandler_WithAuthentication(t *testing.T) {
	extractor := func(_ context.Context, r *http.Request) (rocco.Identity, error) {
		token := r.Header.Get("Authorization")
		if token == "" {
			return nil, nil
		}
		return rtesting.NewMockIdentity("user-123").
			WithScopes("events:read"), nil
	}

	engine := rtesting.TestEngineWithAuth(extractor)

	var receivedIdentity rocco.Identity

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"auth-stream",
		http.MethodGet,
		"/events",
		func(req *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			receivedIdentity = req.Identity
			return stream.Send(streamEvent{Message: "authenticated", Seq: 0})
		},
	).WithAuthentication().WithScopes("events:read")

	engine.WithHandlers(handler)

	// Request with valid auth
	headers := map[string]string{"Authorization": "Bearer token"}
	capture := rtesting.ServeStreamWithHeaders(engine, "GET", "/events", nil, headers)

	if capture.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", capture.Code)
	}

	if receivedIdentity == nil {
		t.Fatal("expected identity to be set")
	}
	if receivedIdentity.ID() != "user-123" {
		t.Errorf("expected identity ID 'user-123', got %q", receivedIdentity.ID())
	}
}

func TestStreamHandler_ValidationError(t *testing.T) {
	engine := rtesting.TestEngine()

	handler := rocco.NewStreamHandler[streamInput, streamEvent](
		"validation-stream",
		http.MethodPost,
		"/events",
		func(_ *rocco.Request[streamInput], _ rocco.Stream[streamEvent]) error {
			t.Error("handler should not be called on validation error")
			return nil
		},
	)

	engine.WithHandlers(handler)

	// Empty topic should fail validation
	capture := rtesting.ServeStream(engine, "POST", "/events", streamInput{Topic: ""})

	if capture.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", capture.Code)
	}
}

func TestStreamHandler_PathParams(t *testing.T) {
	engine := rtesting.TestEngine()

	var receivedChannel string

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"channel-stream",
		http.MethodGet,
		"/channels/{channel}/events",
		func(req *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			receivedChannel = req.Params.Path["channel"]
			return stream.Send(streamEvent{Message: receivedChannel, Seq: 0})
		},
	).WithPathParams("channel")

	engine.WithHandlers(handler)

	capture := rtesting.ServeStream(engine, "GET", "/channels/general/events", nil)

	rtesting.AssertSSE(t, capture)

	if receivedChannel != "general" {
		t.Errorf("expected channel 'general', got %q", receivedChannel)
	}

	events := capture.ParseEvents()
	var first streamEvent
	events[0].DecodeJSON(&first)
	if first.Message != "general" {
		t.Errorf("expected message 'general', got %q", first.Message)
	}
}

func TestStreamHandler_QueryParams(t *testing.T) {
	engine := rtesting.TestEngine()

	var receivedFilter string

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"filter-stream",
		http.MethodGet,
		"/events",
		func(req *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			receivedFilter = req.Params.Query["filter"]
			return stream.Send(streamEvent{Message: receivedFilter, Seq: 0})
		},
	).WithQueryParams("filter")

	engine.WithHandlers(handler)

	capture := rtesting.ServeStream(engine, "GET", "/events?filter=important", nil)

	rtesting.AssertSSE(t, capture)

	if receivedFilter != "important" {
		t.Errorf("expected filter 'important', got %q", receivedFilter)
	}
}

func TestStreamHandler_KeepAliveComments(t *testing.T) {
	engine := rtesting.TestEngine()

	handler := rocco.NewStreamHandler[rocco.NoBody, streamEvent](
		"keepalive-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[streamEvent]) error {
			stream.SendComment("keep-alive")
			stream.Send(streamEvent{Message: "data", Seq: 0})
			stream.SendComment("still-alive")
			return nil
		},
	)

	engine.WithHandlers(handler)

	capture := rtesting.ServeStream(engine, "GET", "/events", nil)

	rtesting.AssertSSE(t, capture)

	body := capture.Body.String()
	if !containsComment(body, "keep-alive") {
		t.Error("expected body to contain keep-alive comment")
	}
	if !containsComment(body, "still-alive") {
		t.Error("expected body to contain still-alive comment")
	}

	// Should still have the data event
	events := capture.ParseEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 data event, got %d", len(events))
	}
}

func containsComment(body, comment string) bool {
	// Comments are filtered out by ParseSSEEvents, so check raw body
	expected := ": " + comment
	for _, line := range splitLines(body) {
		if line == expected {
			return true
		}
	}
	return false
}

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
