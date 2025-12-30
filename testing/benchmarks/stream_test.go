package benchmarks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zoobzio/rocco"
)

type benchStreamEvent struct {
	Message string `json:"message"`
	Seq     int    `json:"seq"`
}

// flushRecorder wraps httptest.ResponseRecorder to implement http.Flusher.
type flushRecorder struct {
	*httptest.ResponseRecorder
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

func (f *flushRecorder) Flush() {}

func BenchmarkStreamHandler_EventThroughput(b *testing.B) {
	engine := rocco.NewEngine("localhost", 0, nil)

	handler := rocco.NewStreamHandler[rocco.NoBody, benchStreamEvent](
		"bench-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[benchStreamEvent]) error {
			for i := 0; i < 100; i++ {
				if err := stream.Send(benchStreamEvent{
					Message: "benchmark",
					Seq:     i,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	engine.WithHandlers(handler)
	router := engine.Router()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/events", nil)
		w := newFlushRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkStreamHandler_ConnectionSetup(b *testing.B) {
	engine := rocco.NewEngine("localhost", 0, nil)

	handler := rocco.NewStreamHandler[rocco.NoBody, benchStreamEvent](
		"bench-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[benchStreamEvent]) error {
			// Send single event to measure setup overhead
			return stream.Send(benchStreamEvent{Message: "setup", Seq: 0})
		},
	)

	engine.WithHandlers(handler)
	router := engine.Router()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/events", nil)
		w := newFlushRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkStreamHandler_NamedEvents(b *testing.B) {
	engine := rocco.NewEngine("localhost", 0, nil)

	handler := rocco.NewStreamHandler[rocco.NoBody, benchStreamEvent](
		"bench-stream",
		http.MethodGet,
		"/events",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[benchStreamEvent]) error {
			for i := 0; i < 100; i++ {
				if err := stream.SendEvent("update", benchStreamEvent{
					Message: "named",
					Seq:     i,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	engine.WithHandlers(handler)
	router := engine.Router()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/events", nil)
		w := newFlushRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkStreamHandler_VsHandler(b *testing.B) {
	type benchOutput struct {
		Items []benchStreamEvent `json:"items"`
	}

	// Create equivalent handler and stream handler
	engine := rocco.NewEngine("localhost", 0, nil)

	// Regular handler returns all items at once
	regularHandler := rocco.NewHandler[rocco.NoBody, benchOutput](
		"bench-regular",
		http.MethodGet,
		"/regular",
		func(_ *rocco.Request[rocco.NoBody]) (benchOutput, error) {
			items := make([]benchStreamEvent, 100)
			for i := 0; i < 100; i++ {
				items[i] = benchStreamEvent{Message: "item", Seq: i}
			}
			return benchOutput{Items: items}, nil
		},
	)

	// Stream handler sends items one by one
	streamHandler := rocco.NewStreamHandler[rocco.NoBody, benchStreamEvent](
		"bench-stream",
		http.MethodGet,
		"/stream",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[benchStreamEvent]) error {
			for i := 0; i < 100; i++ {
				if err := stream.Send(benchStreamEvent{Message: "item", Seq: i}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	engine.WithHandlers(regularHandler, streamHandler)
	router := engine.Router()

	b.Run("Regular", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/regular", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})

	b.Run("Stream", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/stream", nil)
			w := newFlushRecorder()
			router.ServeHTTP(w, req)
		}
	})
}

func BenchmarkStreamHandler_EventSizes(b *testing.B) {
	type smallEvent struct {
		ID int `json:"id"`
	}

	type largeEvent struct {
		ID      int    `json:"id"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}

	engine := rocco.NewEngine("localhost", 0, nil)

	smallHandler := rocco.NewStreamHandler[rocco.NoBody, smallEvent](
		"small-stream",
		http.MethodGet,
		"/small",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[smallEvent]) error {
			for i := 0; i < 100; i++ {
				if err := stream.Send(smallEvent{ID: i}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	largeData := string(make([]byte, 1024)) // 1KB payload

	largeHandler := rocco.NewStreamHandler[rocco.NoBody, largeEvent](
		"large-stream",
		http.MethodGet,
		"/large",
		func(_ *rocco.Request[rocco.NoBody], stream rocco.Stream[largeEvent]) error {
			for i := 0; i < 100; i++ {
				if err := stream.Send(largeEvent{
					ID:      i,
					Message: "large event",
					Data:    largeData,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	engine.WithHandlers(smallHandler, largeHandler)
	router := engine.Router()

	b.Run("Small", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/small", nil)
			w := newFlushRecorder()
			router.ServeHTTP(w, req)
		}
	})

	b.Run("Large", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/large", nil)
			w := newFlushRecorder()
			router.ServeHTTP(w, req)
		}
	})
}
