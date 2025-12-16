package benchmarks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zoobzio/rocco"
)

// Test types for benchmarks.
type simpleInput struct {
	Name string `json:"name"`
}

type simpleOutput struct {
	Message string `json:"message"`
}

type complexInput struct {
	Name     string   `json:"name" validate:"required,min=1,max=100"`
	Email    string   `json:"email" validate:"required,email"`
	Age      int      `json:"age" validate:"gte=0,lte=150"`
	Tags     []string `json:"tags" validate:"max=10"`
	Metadata struct {
		Source    string `json:"source"`
		Version   int    `json:"version"`
		Timestamp int64  `json:"timestamp"`
	} `json:"metadata"`
}

type complexOutput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Age       int      `json:"age"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// newBenchmarkEngine creates a clean engine for benchmarks.
func newBenchmarkEngine() *rocco.Engine {
	return rocco.NewEngine("localhost", 0, nil)
}

// BenchmarkHandler_NoBody benchmarks handlers with no request body.
func BenchmarkHandler_NoBody(b *testing.B) {
	engine := newBenchmarkEngine()
	handler := rocco.NewHandler[rocco.NoBody, simpleOutput](
		"no-body",
		"GET",
		"/test",
		func(_ *rocco.Request[rocco.NoBody]) (simpleOutput, error) {
			return simpleOutput{Message: "hello"}, nil
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_SimpleBody benchmarks handlers with simple JSON body.
func BenchmarkHandler_SimpleBody(b *testing.B) {
	engine := newBenchmarkEngine()
	handler := rocco.NewHandler[simpleInput, simpleOutput](
		"simple-body",
		"POST",
		"/test",
		func(req *rocco.Request[simpleInput]) (simpleOutput, error) {
			return simpleOutput{Message: "hello " + req.Body.Name}, nil
		},
	)
	engine.WithHandlers(handler)

	body, _ := json.Marshal(simpleInput{Name: "benchmark"})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_ComplexBody benchmarks handlers with complex validated JSON body.
func BenchmarkHandler_ComplexBody(b *testing.B) {
	engine := newBenchmarkEngine()
	handler := rocco.NewHandler[complexInput, complexOutput](
		"complex-body",
		"POST",
		"/users",
		func(req *rocco.Request[complexInput]) (complexOutput, error) {
			return complexOutput{
				ID:        "user-123",
				Name:      req.Body.Name,
				Email:     req.Body.Email,
				Age:       req.Body.Age,
				Tags:      req.Body.Tags,
				CreatedAt: "2024-01-01T00:00:00Z",
				UpdatedAt: "2024-01-01T00:00:00Z",
			}, nil
		},
	)
	engine.WithHandlers(handler)

	input := complexInput{
		Name:  "John Doe",
		Email: "john@example.com",
		Age:   30,
		Tags:  []string{"user", "premium", "verified"},
	}
	input.Metadata.Source = "api"
	input.Metadata.Version = 1
	input.Metadata.Timestamp = 1704067200

	body, _ := json.Marshal(input)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_PathParams benchmarks handlers with path parameters.
func BenchmarkHandler_PathParams(b *testing.B) {
	engine := newBenchmarkEngine()
	handler := rocco.NewHandler[rocco.NoBody, simpleOutput](
		"path-params",
		"GET",
		"/users/{id}",
		func(req *rocco.Request[rocco.NoBody]) (simpleOutput, error) {
			return simpleOutput{Message: "user " + req.Params.Path["id"]}, nil
		},
	).WithPathParams("id")
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/users/12345", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_QueryParams benchmarks handlers with query parameters.
func BenchmarkHandler_QueryParams(b *testing.B) {
	engine := newBenchmarkEngine()
	handler := rocco.NewHandler[rocco.NoBody, simpleOutput](
		"query-params",
		"GET",
		"/search",
		func(req *rocco.Request[rocco.NoBody]) (simpleOutput, error) {
			return simpleOutput{Message: "found " + req.Params.Query["q"]}, nil
		},
	).WithQueryParams("q", "limit", "offset")
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/search?q=test&limit=10&offset=0", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_ErrorResponse benchmarks error response generation.
func BenchmarkHandler_ErrorResponse(b *testing.B) {
	engine := newBenchmarkEngine()
	handler := rocco.NewHandler[rocco.NoBody, simpleOutput](
		"error-handler",
		"GET",
		"/error",
		func(_ *rocco.Request[rocco.NoBody]) (simpleOutput, error) {
			return simpleOutput{}, rocco.ErrNotFound.WithMessage("resource not found")
		},
	).WithErrors(rocco.ErrNotFound)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/error", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_Middleware benchmarks handler with middleware chain.
func BenchmarkHandler_Middleware(b *testing.B) {
	engine := newBenchmarkEngine()

	// Simple pass-through middleware
	passthrough := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	handler := rocco.NewHandler[rocco.NoBody, simpleOutput](
		"middleware-handler",
		"GET",
		"/test",
		func(_ *rocco.Request[rocco.NoBody]) (simpleOutput, error) {
			return simpleOutput{Message: "hello"}, nil
		},
	).WithMiddleware(passthrough, passthrough, passthrough)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkHandler_BodySizes benchmarks varying body sizes.
func BenchmarkHandler_BodySizes(b *testing.B) {
	type largeInput struct {
		Data string `json:"data"`
	}

	sizes := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			engine := newBenchmarkEngine()
			handler := rocco.NewHandler[largeInput, simpleOutput](
				"large-body",
				"POST",
				"/data",
				func(_ *rocco.Request[largeInput]) (simpleOutput, error) {
					return simpleOutput{Message: "received"}, nil
				},
			)
			engine.WithHandlers(handler)

			// Generate data of specified size
			data := make([]byte, size.size)
			for i := range data {
				data[i] = 'x'
			}
			body, _ := json.Marshal(largeInput{Data: string(data)})

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/data", bytes.NewReader(body))
				w := httptest.NewRecorder()
				engine.Router().ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkHandler_ValidationComplexity benchmarks validation with varying complexity.
func BenchmarkHandler_ValidationComplexity(b *testing.B) {
	b.Run("NoValidation", func(b *testing.B) {
		type noValInput struct {
			Name string `json:"name"`
		}

		engine := newBenchmarkEngine()
		handler := rocco.NewHandler[noValInput, simpleOutput](
			"no-validation",
			"POST",
			"/test",
			func(_ *rocco.Request[noValInput]) (simpleOutput, error) {
				return simpleOutput{Message: "ok"}, nil
			},
		)
		engine.WithHandlers(handler)

		body, _ := json.Marshal(noValInput{Name: "test"})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})

	b.Run("SimpleValidation", func(b *testing.B) {
		type simpleValInput struct {
			Name string `json:"name" validate:"required"`
		}

		engine := newBenchmarkEngine()
		handler := rocco.NewHandler[simpleValInput, simpleOutput](
			"simple-validation",
			"POST",
			"/test",
			func(_ *rocco.Request[simpleValInput]) (simpleOutput, error) {
				return simpleOutput{Message: "ok"}, nil
			},
		)
		engine.WithHandlers(handler)

		body, _ := json.Marshal(simpleValInput{Name: "test"})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})

	b.Run("ComplexValidation", func(b *testing.B) {
		engine := newBenchmarkEngine()
		handler := rocco.NewHandler[complexInput, simpleOutput](
			"complex-validation",
			"POST",
			"/test",
			func(_ *rocco.Request[complexInput]) (simpleOutput, error) {
				return simpleOutput{Message: "ok"}, nil
			},
		)
		engine.WithHandlers(handler)

		input := complexInput{
			Name:  "John Doe",
			Email: "john@example.com",
			Age:   30,
			Tags:  []string{"a", "b", "c"},
		}
		body, _ := json.Marshal(input)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})
}
