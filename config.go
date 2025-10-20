package rocco

import "time"

// EngineConfig holds configuration for the Engine.
type EngineConfig struct {
	// Server settings
	Host         string        // Host to bind to (e.g., "localhost", "0.0.0.0", or empty for all interfaces)
	Port         int           // Port to listen on (e.g., 8080)
	ReadTimeout  time.Duration // Maximum duration for reading entire request
	WriteTimeout time.Duration // Maximum duration for writing response
	IdleTimeout  time.Duration // Maximum time to wait for next request on keep-alive
}

// DefaultConfig returns an EngineConfig with sensible defaults.
func DefaultConfig() *EngineConfig {
	return &EngineConfig{
		Host:         "", // Empty string binds to all interfaces
		Port:         8080,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// WithHost sets the host to bind to.
func (c *EngineConfig) WithHost(host string) *EngineConfig {
	c.Host = host
	return c
}

// WithPort sets the port to listen on.
func (c *EngineConfig) WithPort(port int) *EngineConfig {
	c.Port = port
	return c
}
