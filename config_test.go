package rocco

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Host != "" {
		t.Errorf("expected empty host, got %q", config.Host)
	}
	if config.Port != 8080 {
		t.Errorf("expected port 8080, got %d", config.Port)
	}
	if config.ReadTimeout != 10*time.Second {
		t.Errorf("expected read timeout 10s, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 10*time.Second {
		t.Errorf("expected write timeout 10s, got %v", config.WriteTimeout)
	}
	if config.IdleTimeout != 120*time.Second {
		t.Errorf("expected idle timeout 120s, got %v", config.IdleTimeout)
	}
}

func TestEngineConfig_WithHost(t *testing.T) {
	config := DefaultConfig().WithHost("localhost")

	if config.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", config.Host)
	}
}

func TestEngineConfig_WithPort(t *testing.T) {
	config := DefaultConfig().WithPort(9000)

	if config.Port != 9000 {
		t.Errorf("expected port 9000, got %d", config.Port)
	}
}

func TestEngineConfig_Chaining(t *testing.T) {
	config := DefaultConfig().
		WithHost("0.0.0.0").
		WithPort(3000)

	if config.Host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got %q", config.Host)
	}
	if config.Port != 3000 {
		t.Errorf("expected port 3000, got %d", config.Port)
	}
}
