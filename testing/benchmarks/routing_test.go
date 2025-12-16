package benchmarks

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/zoobzio/rocco"
)

type emptyOutput struct{}

// BenchmarkRouting_StaticPaths benchmarks routing with static paths.
func BenchmarkRouting_StaticPaths(b *testing.B) {
	counts := []int{1, 10, 50, 100}

	for _, count := range counts {
		b.Run(fmt.Sprintf("%dRoutes", count), func(b *testing.B) {
			engine := newBenchmarkEngine()

			// Register n handlers with static paths
			for i := 0; i < count; i++ {
				path := fmt.Sprintf("/api/v1/resource%d", i)
				handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
					fmt.Sprintf("handler-%d", i),
					"GET",
					path,
					func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
						return emptyOutput{}, nil
					},
				)
				engine.WithHandlers(handler)
			}

			// Target the last registered handler (worst case for linear search)
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/resource%d", count-1), nil)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				engine.Router().ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkRouting_ParamPaths benchmarks routing with path parameters.
func BenchmarkRouting_ParamPaths(b *testing.B) {
	b.Run("SingleParam", func(b *testing.B) {
		engine := newBenchmarkEngine()
		handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
			"single-param",
			"GET",
			"/users/{id}",
			func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
				return emptyOutput{}, nil
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
	})

	b.Run("MultiParam", func(b *testing.B) {
		engine := newBenchmarkEngine()
		handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
			"multi-param",
			"GET",
			"/orgs/{org}/teams/{team}/members/{member}",
			func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
				return emptyOutput{}, nil
			},
		).WithPathParams("org", "team", "member")
		engine.WithHandlers(handler)

		req := httptest.NewRequest("GET", "/orgs/acme/teams/engineering/members/john", nil)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})
}

// BenchmarkRouting_MixedMethods benchmarks routing with multiple HTTP methods.
func BenchmarkRouting_MixedMethods(b *testing.B) {
	engine := newBenchmarkEngine()

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	for _, method := range methods {
		handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
			fmt.Sprintf("%s-handler", method),
			method,
			"/resource",
			func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
				return emptyOutput{}, nil
			},
		)
		engine.WithHandlers(handler)
	}

	b.Run("GET", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/resource", nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})

	b.Run("POST", func(b *testing.B) {
		req := httptest.NewRequest("POST", "/resource", nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})

	b.Run("DELETE", func(b *testing.B) {
		req := httptest.NewRequest("DELETE", "/resource", nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}
	})
}

// BenchmarkRouting_DeepPaths benchmarks routing with varying path depths.
func BenchmarkRouting_DeepPaths(b *testing.B) {
	depths := []int{1, 3, 5, 10}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("Depth%d", depth), func(b *testing.B) {
			engine := newBenchmarkEngine()

			// Build path with specified depth
			path := ""
			reqPath := ""
			for i := 0; i < depth; i++ {
				path += fmt.Sprintf("/level%d", i)
				reqPath += fmt.Sprintf("/level%d", i)
			}

			handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
				"deep-handler",
				"GET",
				path,
				func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
					return emptyOutput{}, nil
				},
			)
			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", reqPath, nil)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				engine.Router().ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkRouting_NotFound benchmarks 404 responses.
func BenchmarkRouting_NotFound(b *testing.B) {
	engine := newBenchmarkEngine()

	// Register a handler to ensure router is initialized
	handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
		"exists",
		"GET",
		"/exists",
		func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
			return emptyOutput{}, nil
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/does-not-exist", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}

// BenchmarkRouting_MethodNotAllowed benchmarks 405 responses.
func BenchmarkRouting_MethodNotAllowed(b *testing.B) {
	engine := newBenchmarkEngine()

	handler := rocco.NewHandler[rocco.NoBody, emptyOutput](
		"get-only",
		"GET",
		"/resource",
		func(_ *rocco.Request[rocco.NoBody]) (emptyOutput, error) {
			return emptyOutput{}, nil
		},
	)
	engine.WithHandlers(handler)

	req := httptest.NewRequest("POST", "/resource", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)
	}
}
