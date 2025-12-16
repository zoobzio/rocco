# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depends on the CVSS v3.0 Rating:

| Version | Supported          | Status |
| ------- | ------------------ | ------ |
| latest  | ✅ | Active development |
| < latest | ❌ | Security fixes only for critical issues |

## Reporting a Vulnerability

We take the security of rocco seriously. If you have discovered a security vulnerability in this project, please report it responsibly.

### How to Report

**Please DO NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via one of the following methods:

1. **GitHub Security Advisories** (Preferred)
   - Go to the [Security tab](https://github.com/zoobzio/rocco/security) of this repository
   - Click "Report a vulnerability"
   - Fill out the form with details about the vulnerability

2. **Email**
   - Send details to the repository maintainer through GitHub profile contact information
   - Use PGP encryption if possible for sensitive details

### What to Include

Please include the following information (as much as you can provide) to help us better understand the nature and scope of the possible issue:

- **Type of issue** (e.g., buffer overflow, SQL injection, cross-site scripting, etc.)
- **Full paths of source file(s)** related to the manifestation of the issue
- **The location of the affected source code** (tag/branch/commit or direct URL)
- **Any special configuration required** to reproduce the issue
- **Step-by-step instructions** to reproduce the issue
- **Proof-of-concept or exploit code** (if possible)
- **Impact of the issue**, including how an attacker might exploit the issue
- **Your name and affiliation** (optional)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt of your vulnerability report within 48 hours
- **Initial Assessment**: Within 7 days, we will provide an initial assessment of the report
- **Resolution Timeline**: We aim to resolve critical issues within 30 days
- **Disclosure**: We will coordinate with you on the disclosure timeline

### Preferred Languages

We prefer all communications to be in English.

## Security Best Practices

When using rocco in your applications, we recommend:

1. **Keep Dependencies Updated**
   ```bash
   go get -u github.com/zoobzio/rocco
   ```

2. **Use Context Properly**
   - Always pass contexts with appropriate timeouts
   - Handle context cancellation in your handlers

3. **Error Handling**
   - Declare all sentinel errors with WithErrors
   - Never ignore errors from handler processing
   - Implement proper fallback mechanisms

4. **Input Validation**
   - Use struct validation tags for all inputs
   - Validate both request body and parameters
   - Sanitize user inputs before processing

5. **Resource Management**
   - Set appropriate timeouts for handlers
   - Implement rate limiting middleware
   - Use circuit breakers for external services

## Production Security Guide

### HTTPS

Rocco does not handle TLS directly. In production, use one of these approaches:

1. **Reverse Proxy** (Recommended): Use nginx, Caddy, or a cloud load balancer to terminate TLS
2. **TLS in Go**: Wrap the server with `http.ListenAndServeTLS()` (not directly supported by rocco)

```go
// Example: Using behind nginx/Caddy that handles TLS
engine := rocco.NewEngine("127.0.0.1", 8080, extractIdentity)
// nginx forwards https://api.example.com -> http://127.0.0.1:8080
```

### CORS

Rocco doesn't include CORS middleware. Use Chi's cors middleware:

```go
import "github.com/go-chi/cors"

engine := rocco.NewEngine("localhost", 8080, nil)
engine.WithMiddleware(cors.Handler(cors.Options{
    AllowedOrigins:   []string{"https://example.com"},
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
    ExposedHeaders:   []string{"Link"},
    AllowCredentials: true,
    MaxAge:           300,
}))
```

### Identity Extraction

The `extractIdentity` callback is critical for security. Guidelines:

```go
func extractIdentity(ctx context.Context, r *http.Request) (rocco.Identity, error) {
    // 1. Extract token from header
    token := r.Header.Get("Authorization")
    if token == "" {
        return nil, errors.New("missing authorization header")
    }

    // 2. Validate token (JWT verification, session lookup, etc.)
    claims, err := validateToken(strings.TrimPrefix(token, "Bearer "))
    if err != nil {
        return nil, err // Returns 401 Unauthorized
    }

    // 3. Return identity with scopes/roles for authorization
    return &UserIdentity{
        ID:     claims.Subject,
        Scopes: claims.Scopes,
        Roles:  claims.Roles,
    }, nil
}
```

**Important:**
- Never trust client-provided identity claims without verification
- Use constant-time comparison for token validation
- Set reasonable token expiration times
- Log authentication failures for security monitoring

### Request Size Limits

Rocco enforces a 10MB default body size limit. Adjust per-handler:

```go
handler.WithMaxBodySize(1 * 1024 * 1024) // 1MB limit
```

Requests exceeding the limit return 413 Payload Too Large.

### Rate Limiting

Implement rate limiting at the middleware level:

```go
import "github.com/go-chi/httprate"

engine.WithMiddleware(httprate.LimitByIP(100, time.Minute))
```

Or use rocco's built-in usage limits for authenticated routes:

```go
handler.WithUsageLimit("api_calls", func(id rocco.Identity) int {
    if id.HasRole("premium") {
        return 10000
    }
    return 100
})
```

### SQL Injection Prevention

Rocco validates JSON input but doesn't protect against SQL injection. Always use parameterized queries:

```go
// WRONG - vulnerable
db.Query("SELECT * FROM users WHERE id = " + req.Params.Path["id"])

// CORRECT - parameterized
db.Query("SELECT * FROM users WHERE id = $1", req.Params.Path["id"])
```

## Security Features

rocco includes several built-in security features:

- **Type Safety**: Generic types prevent type confusion attacks
- **Context Support**: Built-in cancellation and timeout support
- **Error Isolation**: Sentinel errors are properly tracked and reported
- **Input Validation**: Automatic struct validation with detailed error messages
- **Observability**: Built-in metrics and tracing for security monitoring

## Automated Security Scanning

This project uses:

- **CodeQL**: GitHub's semantic code analysis for security vulnerabilities
- **Dependabot**: Automated dependency updates
- **golangci-lint**: Static analysis including security linters
- **Codecov**: Coverage tracking to ensure security-critical code is tested

## Vulnerability Disclosure Policy

- Security vulnerabilities will be disclosed via GitHub Security Advisories
- We follow a 90-day disclosure timeline for non-critical issues
- Critical vulnerabilities may be disclosed sooner after patches are available
- We will credit reporters who follow responsible disclosure practices

## Credits

We thank the following individuals for responsibly disclosing security issues:

_This list is currently empty. Be the first to help improve our security!_

---

**Last Updated**: 2025-10-15
