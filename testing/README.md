# Testing

This directory contains comprehensive tests for rocco, organized into benchmarks and integration tests that validate real-world behavior.

## Structure

```
testing/
├── helpers.go              # Reusable test utilities
├── helpers_test.go         # Tests for helpers
├── benchmarks/
│   ├── handler_test.go     # Handler processing benchmarks
│   ├── routing_test.go     # Route matching and middleware
│   └── openapi_test.go     # OpenAPI spec generation
└── integration/
    ├── concurrency_test.go # Concurrent request handling
    ├── lifecycle_test.go   # Full request lifecycle
    └── real_world_test.go  # Real-world API scenarios
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run only benchmarks
go test ./testing/benchmarks/... -bench=. -benchmem

# Run only integration tests
go test ./testing/integration/...

# Run with verbose output
go test ./testing/... -v

# Run with race detector
go test ./testing/... -race
```

## Test Helpers

The `helpers.go` file provides utilities for writing tests:

### ResponseCapture

Captures HTTP responses with convenient access methods:

```go
capture := NewResponseCapture()
engine.mux.ServeHTTP(capture, req)

if capture.StatusCode() != 200 {
    t.Errorf("expected 200, got %d", capture.StatusCode())
}

var output MyType
capture.DecodeJSON(&output)
```

### RequestBuilder

Fluent builder for constructing test requests:

```go
req := NewRequestBuilder("POST", "/users").
    WithJSON(CreateUserInput{Name: "test"}).
    WithHeader("Authorization", "Bearer token").
    Build()
```

### TestEngine

Pre-configured engine for testing with sensible defaults:

```go
engine := NewTestEngine()
engine.WithHandlers(myHandler)

// Make requests
capture := ServeRequest(engine, "GET", "/test", nil)
```

## Benchmarks

Benchmarks measure performance characteristics:

- **Handler Processing**: Time to parse, validate, and execute handlers
- **Routing**: Route matching with various path patterns
- **OpenAPI Generation**: Spec generation with different numbers of handlers

Run with memory allocation stats:

```bash
go test ./testing/benchmarks/... -bench=. -benchmem -count=5
```

## Integration Tests

Integration tests verify complete request flows:

- **Concurrency**: Multiple simultaneous requests, race condition detection
- **Lifecycle**: Authentication, authorization, error handling flows
- **Real-World**: CRUD operations, multi-step workflows

Run with race detector enabled:

```bash
go test ./testing/integration/... -race -v
```

## Writing New Tests

### Benchmark Guidelines

1. Use `b.ReportAllocs()` to track allocations
2. Use sub-benchmarks with `b.Run()` for variations
3. Reset timer after setup: `b.ResetTimer()`
4. Run operations in a loop: `for i := 0; i < b.N; i++`

### Integration Test Guidelines

1. Test complete request flows, not isolated units
2. Use real HTTP requests via httptest
3. Verify both success and error paths
4. Include concurrent access patterns
5. Clean up resources in defer blocks
