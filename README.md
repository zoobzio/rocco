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
- **Observability**: Built-in metrics, tracing, and hooks
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

    engine.Register(handler)

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

Rocco uses Chi middleware - add any Chi-compatible middleware:

```go
import (
    "github.com/go-chi/chi/v5/middleware"
)

engine := rocco.NewEngine(rocco.DefaultConfig())

// Add middleware before registering handlers
engine.Use(middleware.Logger)
engine.Use(middleware.Recoverer)
engine.Use(middleware.RequestID)

// Register handlers
engine.Register(handler)
```

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

Enrich your OpenAPI documentation with struct tags. Rocco extracts these tags automatically via [sentinel](https://github.com/zoobzio/sentinel):

```go
type CreateUserInput struct {
    Name  string `json:"name" description:"User's full name" example:"John Doe" minLength:"2" maxLength:"50"`
    Email string `json:"email" description:"User email address" format:"email" example:"user@example.com" pattern:"^[^@]+@[^@]+\.[^@]+$"`
    Age   int    `json:"age" description:"User age in years" minimum:"18" maximum:"120" example:"25"`
    Role  string `json:"role" description:"User role" enum:"admin,user,guest" example:"user"`
}
```

**Supported tags:**

| Tag | Type | Description | Example |
|-----|------|-------------|---------|
| `description` | string | Field description | `description:"User's full name"` |
| `example` | any | Example value | `example:"John Doe"` |
| `format` | string | Data format (e.g., email, uri, date-time) | `format:"email"` |
| `pattern` | string | Regex pattern | `pattern:"^[a-z]+$"` |
| `enum` | string | Comma-separated allowed values | `enum:"red,green,blue"` |
| `minimum` | number | Minimum numeric value | `minimum:"0"` |
| `maximum` | number | Maximum numeric value | `maximum:"100"` |
| `multipleOf` | number | Number must be multiple of | `multipleOf:"5"` |
| `minLength` | integer | Minimum string length | `minLength:"3"` |
| `maxLength` | integer | Maximum string length | `maxLength:"50"` |
| `minItems` | integer | Minimum array items | `minItems:"1"` |
| `maxItems` | integer | Maximum array items | `maxItems:"10"` |
| `uniqueItems` | boolean | Array items must be unique | `uniqueItems:"true"` |
| `nullable` | boolean | Value can be null | `nullable:"true"` |
| `readOnly` | boolean | Read-only field | `readOnly:"true"` |
| `writeOnly` | boolean | Write-only field | `writeOnly:"true"` |
| `deprecated` | boolean | Field is deprecated | `deprecated:"true"` |

**Examples are type-aware:**
- String fields: `example:"hello"` → `"hello"`
- Integer fields: `example:"42"` → `42`
- Number fields: `example:"3.14"` → `3.14`
- Boolean fields: `example:"true"` → `true`
- Array fields: `example:"a,b,c"` → `["a", "b", "c"]`

**Enum values are type-aware:**
- String: `enum:"red,green,blue"` → `["red", "green", "blue"]`
- Integer: `enum:"1,2,3"` → `[1, 2, 3]`
- Number: `enum:"1.5,2.5,3.5"` → `[1.5, 2.5, 3.5]`

### Observability

Built-in metrics, tracing, and hooks:

```go
// Access metrics
engine.Metrics().Counter("custom.metric").Inc()

// Access tracer
ctx, span := engine.Tracer().StartSpan(ctx, "operation")
defer span.Finish()

// Register hooks
engine.OnRequestReceived(func(ctx context.Context, event *rocco.Event) error {
    fmt.Println("Request received:", event.Type)
    return nil
})

engine.OnRequestCompleted(func(ctx context.Context, event *rocco.Event) error {
    fmt.Println("Request completed")
    return nil
})

engine.OnRequestRejected(func(ctx context.Context, event *rocco.Event) error {
    fmt.Println("Request rejected:", event.Data["error"])
    return nil
})
```

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
- [metricz](https://github.com/zoobzio/metricz) - Metrics collection
- [tracez](https://github.com/zoobzio/tracez) - Distributed tracing
- [hookz](https://github.com/zoobzio/hookz) - Event hooks

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

See [SECURITY.md](SECURITY.md) for security policy and reporting vulnerabilities.

## License

MIT License - see [LICENSE](LICENSE) for details.
