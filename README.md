# rocco

[![CI Status](https://github.com/zoobzio/rocco/workflows/CI/badge.svg)](https://github.com/zoobzio/rocco/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/rocco/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/rocco)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/rocco)](https://goreportcard.com/report/github.com/zoobzio/rocco)
[![CodeQL](https://github.com/zoobzio/rocco/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/rocco/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/rocco.svg)](https://pkg.go.dev/github.com/zoobzio/rocco)
[![License](https://img.shields.io/github/license/zoobzio/rocco)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/rocco)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/rocco)](https://github.com/zoobzio/rocco/releases)

Type-safe HTTP framework for Go with automatic OpenAPI generation.

## Features

- **Type-Safe Handlers**: Generic handlers with compile-time type checking
- **Automatic OpenAPI**: Generate OpenAPI 3.0.3 specs from your code
- **Request Validation**: Built-in validation using struct tags
- **Sentinel Errors**: HTTP error handling with sentinel error pattern
- **Chi Integration**: Powered by the battle-tested Chi router
- **Zero Magic**: Explicit configuration, no hidden behaviors

## Installation

```bash
go get github.com/zoobzio/rocco
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/zoobzio/rocco"
)

type CreateUserInput struct {
    Name  string `json:"name" validate:"required,min=2"`
    Email string `json:"email" validate:"required,email"`
}

type UserOutput struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    // Create engine (host, port, identity extractor)
    // Pass nil for identity extractor if not using authentication
    engine := rocco.NewEngine("", 8080, nil)

    // Register handler
    handler := rocco.NewHandler[CreateUserInput, UserOutput](
        "create-user",
        "POST",
        "/users",
        func(req *rocco.Request[CreateUserInput]) (UserOutput, error) {
            // Your business logic here
            return UserOutput{
                ID:    "usr_123",
                Name:  req.Body.Name,
                Email: req.Body.Email,
            }, nil
        },
    ).
        WithSummary("Create a new user").
        WithDescription("Creates a new user account").
        WithTags("users").
        WithSuccessStatus(201).
        WithErrors(rocco.ErrBadRequest, rocco.ErrUnprocessableEntity)

    engine.WithHandlers(handler)

    // Optional: OpenAPI spec endpoint
    engine.RegisterOpenAPIHandler("/openapi.json", rocco.Info{
        Title:   "User API",
        Version: "1.0.0",
    })

    // Start server
    fmt.Println("Server listening on :8080")
    engine.Start()
}
```

## Core Concepts

### Handlers

Handlers are type-safe request processors defined with generic input and output types:

```go
// Handler with body input
handler := rocco.NewHandler[CreateUserInput, UserOutput](
    "create-user",
    "POST",
    "/users",
    func(req *rocco.Request[CreateUserInput]) (UserOutput, error) {
        // Access validated body via req.Body
        return UserOutput{...}, nil
    },
)

// Handler with no body (GET requests)
handler := rocco.NewHandler[rocco.NoBody, UserListOutput](
    "list-users",
    "GET",
    "/users",
    func(req *rocco.Request[rocco.NoBody]) (UserListOutput, error) {
        // Access query params via req.Params.Query
        return UserListOutput{...}, nil
    },
).WithQueryParams("page", "limit")
```

### Request Parameters

Access path and query parameters through the `Request.Params` field:

```go
handler := rocco.NewHandler[rocco.NoBody, UserOutput](
    "get-user",
    "GET",
    "/users/{id}",
    func(req *rocco.Request[rocco.NoBody]) (UserOutput, error) {
        userID := req.Params.Path["id"]
        page := req.Params.Query["page"]

        // Your logic here
        return UserOutput{...}, nil
    },
).
    WithPathParams("id").
    WithQueryParams("page", "limit")
```

### Validation

Validation is automatic using struct tags from `go-playground/validator`:

```go
type CreateUserInput struct {
    Name  string `json:"name" validate:"required,min=2,max=50"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"required,min=18,max=120"`
}

// Invalid inputs automatically return 422 with detailed error messages
```

### Error Handling

Use sentinel errors for HTTP error responses. Errors are typed with generic details for comprehensive OpenAPI generation:

```go
func(req *rocco.Request[GetUserInput]) (UserOutput, error) {
    user, err := db.GetUser(req.Params.Path["id"])
    if err != nil {
        // Simple sentinel error
        return UserOutput{}, rocco.ErrNotFound

        // With custom message
        return UserOutput{}, rocco.ErrNotFound.WithMessage("user not found")

        // With typed details (for OpenAPI schema generation)
        return UserOutput{}, rocco.ErrNotFound.WithDetails(rocco.NotFoundDetails{
            Resource: "user",
        })
    }
    return UserOutput{...}, nil
}

// Must declare errors in handler
handler.WithErrors(rocco.ErrNotFound)
```

Available sentinel errors (all with typed details for OpenAPI):
- `ErrBadRequest` (400) - `BadRequestDetails`
- `ErrUnauthorized` (401) - `UnauthorizedDetails`
- `ErrForbidden` (403) - `ForbiddenDetails`
- `ErrNotFound` (404) - `NotFoundDetails`
- `ErrConflict` (409) - `ConflictDetails`
- `ErrPayloadTooLarge` (413) - `PayloadTooLargeDetails`
- `ErrUnprocessableEntity` (422) - `UnprocessableEntityDetails`
- `ErrValidationFailed` (422) - `ValidationDetails`
- `ErrTooManyRequests` (429) - `TooManyRequestsDetails`
- `ErrInternalServer` (500) - `InternalServerDetails`
- `ErrNotImplemented` (501) - `NotImplementedDetails`
- `ErrServiceUnavailable` (503) - `ServiceUnavailableDetails`

**Defining Custom Errors:**

```go
type TeapotDetails struct {
    TeaType string `json:"tea_type" description:"The type of tea"`
}

var ErrTeapot = rocco.NewError[TeapotDetails]("TEAPOT", 418, "I'm a teapot")

// Use in handler
return Output{}, ErrTeapot.WithDetails(TeapotDetails{TeaType: "Earl Grey"})
```

**Important**: You must declare errors with `WithErrors()`. Undeclared sentinel errors will return 500 and log an error.

### Middleware

Rocco uses Chi middleware - add any Chi-compatible middleware at the engine or handler level:

```go
import (
    "github.com/go-chi/chi/v5/middleware"
)

engine := rocco.NewEngine("", 8080, nil)

// Engine-level middleware (applies to all handlers)
engine.WithMiddleware(middleware.Logger)
engine.WithMiddleware(middleware.Recoverer)
engine.WithMiddleware(middleware.RequestID)

// Handler-level middleware (applies to specific handler only)
handler := rocco.NewHandler[CreateUserInput, UserOutput](
    "create-user",
    "POST",
    "/users",
    func(req *rocco.Request[CreateUserInput]) (UserOutput, error) {
        return UserOutput{...}, nil
    },
).
    Use(middleware.AllowContentType("application/json")).
    Use(customAuthMiddleware)

// Register handlers
engine.WithHandlers(handler)
```

Middleware execution order: engine middleware runs first, then handler middleware, then the handler function.

### OpenAPI Generation

OpenAPI 3.0.3 specs are automatically generated from your handlers:

```go
// Register OpenAPI endpoint
engine.RegisterOpenAPIHandler("/openapi.json", rocco.Info{
    Title:       "My API",
    Version:     "1.0.0",
    Description: "API description",
})

// Access at http://localhost:8080/openapi.json
```

The spec includes:
- Request/response schemas from your types
- Path parameters
- Query parameters
- Request body validation rules
- Error responses
- Tags and descriptions

#### OpenAPI Schema Tags

Rocco automatically generates OpenAPI schemas from your types. Use the `validate` tag for runtime validation that also drives OpenAPI constraints, and documentation tags for additional metadata:

```go
type CreateUserInput struct {
    Name  string `json:"name" validate:"min=2,max=50" description:"User's full name" example:"John Doe"`
    Email string `json:"email" validate:"email,min=5,max=100" description:"User email address" example:"user@example.com"`
    Age   int    `json:"age" validate:"min=18,max=120" description:"User age in years" example:"25"`
    Role  string `json:"role" validate:"oneof=admin user guest" description:"User role" example:"user"`
    Tags  []string `json:"tags" validate:"len=5,unique" description:"User tags"`
}
```

**Validate Tag (Runtime Validation + OpenAPI):**

The `validate` tag uses [go-playground/validator](https://github.com/go-playground/validator) syntax and automatically generates corresponding OpenAPI constraints:

| Validator | Applies To | OpenAPI Mapping | Example |
|-----------|------------|-----------------|---------|
| `min=N` | numbers | `minimum` | `validate:"min=0"` |
| `max=N` | numbers | `maximum` | `validate:"max=100"` |
| `min=N` | strings | `minLength` | `validate:"min=3"` |
| `max=N` | strings | `maxLength` | `validate:"max=50"` |
| `gte=N` | numbers | `minimum` | `validate:"gte=0"` |
| `lte=N` | numbers | `maximum` | `validate:"lte=100"` |
| `gt=N` | numbers | `minimum` + `exclusiveMinimum` | `validate:"gt=0"` |
| `lt=N` | numbers | `maximum` + `exclusiveMaximum` | `validate:"lt=100"` |
| `len=N` | arrays | `minItems` + `maxItems` | `validate:"len=5"` |
| `len=N` | strings | `minLength` + `maxLength` | `validate:"len=10"` |
| `unique` | arrays | `uniqueItems` | `validate:"unique"` |
| `email` | strings | `format: "email"` | `validate:"email"` |
| `url` | strings | `format: "uri"` | `validate:"url"` |
| `uuid`, `uuid4`, `uuid5` | strings | `format: "uuid"` | `validate:"uuid4"` |
| `datetime` | strings | `format: "date-time"` | `validate:"datetime"` |
| `ipv4` | strings | `format: "ipv4"` | `validate:"ipv4"` |
| `ipv6` | strings | `format: "ipv6"` | `validate:"ipv6"` |
| `oneof=a b c` | any | `enum: ["a", "b", "c"]` | `validate:"oneof=red green blue"` |

**Documentation Tags:**

| Tag | Description | Example |
|-----|-------------|---------|
| `description` | Field description for OpenAPI | `description:"User's full name"` |
| `example` | Example value (type-aware) | `example:"John Doe"` |

**Benefits:**
- **Single source of truth**: One tag for both validation and documentation
- **Runtime enforcement**: Constraints are validated at runtime
- **Automatic OpenAPI sync**: Documentation always matches validation rules
- **Standard Go practices**: Uses the industry-standard validator library

**Examples are type-aware:**
- String fields: `example:"hello"` → `"hello"`
- Integer fields: `example:"42"` → `42`
- Number fields: `example:"3.14"` → `3.14`
- Boolean fields: `example:"true"` → `true`
- Array fields: `example:"a,b,c"` → `["a", "b", "c"]`

### Observability

Rocco emits lifecycle events via [capitan](https://github.com/zoobzio/capitan), a type-safe event coordination library. Users control observability by hooking events and wiring them to their preferred backends (OpenTelemetry, Prometheus, logging, etc.).

**Hook events for logging:**
```go
import "github.com/zoobzio/capitan"

capitan.Hook(rocco.RequestReceived, func(ctx context.Context, e *capitan.Event) {
    method, _ := rocco.MethodKey.From(e)
    path, _ := rocco.PathKey.From(e)
    log.Printf("Request: %s %s", method, path)
})
```

**Observe all events (metrics, tracing, etc.):**
```go
capitan.Observe(func(ctx context.Context, e *capitan.Event) {
    // Wire to your observability backend
    // (OpenTelemetry, Prometheus, DataDog, etc.)
})
```

**Available Events:**

| Signal | Description | Fields |
|--------|-------------|--------|
| `EngineCreated` (`http.engine.created`) | Engine instance created | `host`, `port` |
| `EngineStarting` (`http.engine.starting`) | Server starting to listen | `host`, `port`, `address` |
| `EngineShutdownStarted` (`http.engine.shutdown.started`) | Shutdown initiated | - |
| `EngineShutdownComplete` (`http.engine.shutdown.complete`) | Shutdown finished | `graceful`, `error` (if failed) |
| `HandlerRegistered` (`http.handler.registered`) | Handler registered with engine | `handler_name`, `method`, `path` |
| `RequestReceived` (`http.request.received`) | Request received | `method`, `path`, `handler_name` |
| `RequestCompleted` (`http.request.completed`) | Request succeeded | `method`, `path`, `handler_name`, `status_code`, `duration_ms` |
| `RequestFailed` (`http.request.failed`) | Request failed | `method`, `path`, `handler_name`, `status_code`, `duration_ms`, `error` |
| `HandlerExecuting` (`http.handler.executing`) | Handler function starting | `handler_name` |
| `HandlerSuccess` (`http.handler.success`) | Handler returned successfully | `handler_name`, `status_code` |
| `HandlerError` (`http.handler.error`) | Handler returned error | `handler_name`, `error` |
| `HandlerSentinelError` (`http.handler.sentinel.error`) | Declared sentinel error returned | `handler_name`, `error`, `status_code` |
| `HandlerUndeclaredSentinel` (`http.handler.sentinel.undeclared`) | Undeclared sentinel error (bug) | `handler_name`, `error`, `status_code` |
| `RequestParamsInvalid` (`http.request.params.invalid`) | Path/query param extraction failed | `handler_name`, `error` |
| `RequestBodyReadError` (`http.request.body.read.error`) | Failed to read request body | `handler_name`, `error` |
| `RequestBodyParseError` (`http.request.body.parse.error`) | Failed to parse JSON body | `handler_name`, `error` |
| `RequestValidationInputFailed` (`http.request.validation.input.failed`) | Input validation failed | `handler_name`, `error` |
| `RequestValidationOutputFailed` (`http.request.validation.output.failed`) | Output validation failed | `handler_name`, `error` |
| `RequestResponseMarshalError` (`http.request.response.marshal.error`) | Failed to marshal response | `handler_name`, `error` |

**Field Keys:**
- `HostKey` (string) - Server host
- `PortKey` (int) - Server port
- `AddressKey` (string) - Server address (host:port)
- `MethodKey` (string) - HTTP method
- `PathKey` (string) - Request path
- `HandlerNameKey` (string) - Handler identifier
- `StatusCodeKey` (int) - HTTP status code
- `DurationMsKey` (int64) - Request duration in milliseconds
- `ErrorKey` (string) - Error message
- `GracefulKey` (bool) - Graceful shutdown success

All events and keys are exported constants in the `rocco` package for type-safe access.

### Configuration

Create an engine with host, port, and identity extractor:

```go
// Basic engine on all interfaces, port 8080, no auth
engine := rocco.NewEngine("", 8080, nil)

// Engine on specific host/port with auth
engine := rocco.NewEngine("0.0.0.0", 3000, extractIdentity)
```

The engine uses these default timeouts:
- ReadTimeout: 120 seconds
- WriteTimeout: 120 seconds
- IdleTimeout: 120 seconds

### Graceful Shutdown

```go
// Handle shutdown signals
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    engine.Start()
}()

<-sigChan
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := engine.Shutdown(ctx); err != nil {
    log.Fatal(err)
}
```

## Development

```bash
# Run tests
make test

# Run linters
make lint

# Generate coverage report
make coverage

# Run all checks (CI simulation)
make ci
```

## Architecture

Rocco is built on:
- [chi](https://github.com/go-chi/chi) - HTTP router
- [sentinel](https://github.com/zoobzio/sentinel) - Type metadata extraction
- [validator](https://github.com/go-playground/validator) - Struct validation
- [capitan](https://github.com/zoobzio/capitan) - Event coordination

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

See [SECURITY.md](SECURITY.md) for security policy and reporting vulnerabilities.

## License

MIT License - see [LICENSE](LICENSE) for details.
