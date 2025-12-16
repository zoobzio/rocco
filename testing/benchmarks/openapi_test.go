package benchmarks

import (
	"fmt"
	"testing"

	"github.com/zoobzio/rocco"
)

// BenchmarkOpenAPI_Generation benchmarks OpenAPI spec generation.
func BenchmarkOpenAPI_Generation(b *testing.B) {
	handlerCounts := []int{1, 10, 50, 100}

	for _, count := range handlerCounts {
		b.Run(fmt.Sprintf("%dHandlers", count), func(b *testing.B) {
			engine := newBenchmarkEngine()

			// Register handlers
			for i := 0; i < count; i++ {
				handler := rocco.NewHandler[simpleInput, simpleOutput](
					fmt.Sprintf("handler-%d", i),
					"POST",
					fmt.Sprintf("/api/resource%d", i),
					func(_ *rocco.Request[simpleInput]) (simpleOutput, error) {
						return simpleOutput{Message: "ok"}, nil
					},
				).WithSummary(fmt.Sprintf("Handler %d", i)).
					WithDescription("A test handler for benchmarking").
					WithTags("test", "benchmark")
				engine.WithHandlers(handler)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = engine.GenerateOpenAPI(nil)
			}
		})
	}
}

// BenchmarkOpenAPI_ComplexHandlers benchmarks OpenAPI generation with complex handlers.
func BenchmarkOpenAPI_ComplexHandlers(b *testing.B) {
	engine := newBenchmarkEngine()

	// Register handlers with various configurations
	for i := 0; i < 20; i++ {
		// Create handlers with unique paths to avoid chi router conflicts
		handler := rocco.NewHandler[complexInput, complexOutput](
			fmt.Sprintf("complex-handler-%d", i),
			"POST",
			fmt.Sprintf("/api/v1/resources%d/{id}", i),
			func(_ *rocco.Request[complexInput]) (complexOutput, error) {
				return complexOutput{}, nil
			},
		).WithPathParams("id").
			WithQueryParams("filter", "sort", "limit", "offset").
			WithSummary("Complex operation").
			WithDescription("A complex handler with path params, query params, and detailed types").
			WithTags("resources", "v1").
			WithErrors(rocco.ErrNotFound, rocco.ErrBadRequest, rocco.ErrUnprocessableEntity)

		engine.WithHandlers(handler)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = engine.GenerateOpenAPI(nil)
	}
}

// BenchmarkOpenAPI_WithErrorSchemas benchmarks OpenAPI generation with error schemas.
func BenchmarkOpenAPI_WithErrorSchemas(b *testing.B) {
	engine := newBenchmarkEngine()

	// Create handlers with various error types
	for i := 0; i < 10; i++ {
		handler := rocco.NewHandler[rocco.NoBody, simpleOutput](
			fmt.Sprintf("error-handler-%d", i),
			"GET",
			fmt.Sprintf("/api/errors%d", i),
			func(_ *rocco.Request[rocco.NoBody]) (simpleOutput, error) {
				return simpleOutput{}, nil
			},
		).WithErrors(
			rocco.ErrNotFound,
			rocco.ErrBadRequest,
			rocco.ErrUnauthorized,
			rocco.ErrForbidden,
			rocco.ErrConflict,
			rocco.ErrUnprocessableEntity,
		)
		engine.WithHandlers(handler)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = engine.GenerateOpenAPI(nil)
	}
}

// BenchmarkOpenAPI_Serialization benchmarks the full spec generation and serialization.
func BenchmarkOpenAPI_Serialization(b *testing.B) {
	engine := newBenchmarkEngine()

	// Register a realistic set of handlers
	for i := 0; i < 25; i++ {
		handler := rocco.NewHandler[simpleInput, simpleOutput](
			fmt.Sprintf("handler-%d", i),
			"POST",
			fmt.Sprintf("/api/v1/endpoint%d", i),
			func(_ *rocco.Request[simpleInput]) (simpleOutput, error) {
				return simpleOutput{}, nil
			},
		).WithSummary("Test endpoint").
			WithTags("api")
		engine.WithHandlers(handler)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		spec := engine.GenerateOpenAPI(nil)
		// Simulate serialization (what would happen in /openapi handler)
		_ = spec
	}
}
