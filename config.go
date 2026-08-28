// Package rocco provides a type-safe HTTP framework for Go with automatic OpenAPI generation.
package rocco

import "time"

// Common host constants for use with Engine.Start().
const (
	HostAll      = ""          // Bind to all interfaces (0.0.0.0)
	HostLocal    = "localhost" // Bind to loopback (localhost)
	HostLoopback = "127.0.0.1" // Bind to loopback (127.0.0.1)
)

// EngineConfig holds configuration for the Engine.
// Set values via Engine.WithTimeouts; NewEngine applies the defaults.
type EngineConfig struct {
	ReadTimeout  time.Duration // Maximum duration for reading entire request
	WriteTimeout time.Duration // Maximum duration for writing response (streams are exempt)
	IdleTimeout  time.Duration // Maximum time to wait for next request on keep-alive
}
