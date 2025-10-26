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
    // Create engine
    engine := rocco.NewEngine(rocco.DefaultConfig())

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
        WithErrorCodes(400, 422)

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

Use sentinel errors for HTTP error responses:

```go
func(req *rocco.Request[GetUserInput]) (UserOutput, error) {
    user, err := db.GetUser(req.Params.Path["id"])
    if err != nil {
        // Return sentinel error for 404
        return UserOutput{}, rocco.ErrNotFound
    }
    return UserOutput{...}, nil
}

// Must declare error codes in handler
handler.WithErrorCodes(404)
```

Available sentinel errors:
- `ErrBadRequest` (400)
- `ErrUnauthorized` (401)
- `ErrForbidden` (403)
- `ErrNotFound` (404)
- `ErrConflict` (409)
- `ErrUnprocessableEntity` (422)
- `ErrTooManyRequests` (429)

**Important**: You must declare error codes with `WithErrorCodes()`. Undeclared sentinel errors will return 500 and log an error.

### Middleware

Rocco uses Chi middleware - add any Chi-compatible middleware at the engine or handler level:

```go
import (
    "github.com/go-chi/chi/v5/middleware"
)

engine := rocco.NewEngine(rocco.DefaultConfig())

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

capitan.Hook(rocco.EventRequestReceived, func(ctx context.Context, e *capitan.Event) {
    method, _ := rocco.KeyMethod.From(e)
    path, _ := rocco.KeyPath.From(e)
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
| `EventEngineCreated` | Engine instance created | `host`, `port` |
| `EventEngineStarting` | Server starting to listen | `host`, `port`, `address` |
| `EventEngineShutdownStarted` | Shutdown initiated | - |
| `EventEngineShutdownComplete` | Shutdown finished | `graceful`, `error` (if failed) |
| `EventHandlerRegistered` | Handler registered with engine | `handler_name`, `method`, `path` |
| `EventRequestReceived` | Request received | `method`, `path`, `handler_name` |
| `EventRequestCompleted` | Request succeeded | `method`, `path`, `handler_name` |
| `EventRequestFailed` | Request failed | `method`, `path`, `handler_name`, `error` |
| `EventHandlerExecuting` | Handler function starting | `handler_name` |
| `EventHandlerSuccess` | Handler returned successfully | `handler_name`, `status_code` |
| `EventHandlerError` | Handler returned error | `handler_name`, `error` |
| `EventHandlerSentinelError` | Declared sentinel error returned | `handler_name`, `error`, `status_code` |
| `EventHandlerUndeclaredSentinel` | Undeclared sentinel error (bug) | `handler_name`, `error`, `status_code` |
| `EventRequestParamsInvalid` | Path/query param extraction failed | `handler_name`, `error` |
| `EventRequestBodyReadError` | Failed to read request body | `handler_name`, `error` |
| `EventRequestBodyParseError` | Failed to parse JSON body | `handler_name`, `error` |
| `EventRequestValidationInputFailed` | Input validation failed | `handler_name`, `error` |
| `EventRequestValidationOutputFailed` | Output validation failed | `handler_name`, `error` |
| `EventRequestResponseMarshalError` | Failed to marshal response | `handler_name`, `error` |

**Field Keys:**
- `KeyHost` (string) - Server host
- `KeyPort` (int) - Server port
- `KeyAddress` (string) - Server address (host:port)
- `KeyMethod` (string) - HTTP method
- `KeyPath` (string) - Request path
- `KeyHandlerName` (string) - Handler identifier
- `KeyStatusCode` (int) - HTTP status code
- `KeyError` (string) - Error message
- `KeyGraceful` (bool) - Graceful shutdown success

All events and keys are exported constants in the `rocco` package for type-safe access.

### Configuration

Customize engine behavior:

```go
config := rocco.NewEngineConfig().
    WithHost("0.0.0.0").
    WithPort(3000).
    WithReadTimeout(10 * time.Second).
    WithWriteTimeout(10 * time.Second).
    WithIdleTimeout(60 * time.Second)

engine := rocco.NewEngine(config)
```

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

## Examples

See the `examples/` directory for complete working examples (coming soon).

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
