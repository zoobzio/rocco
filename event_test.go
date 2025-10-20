package rocco

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRequestEvent(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com"

	metadata := map[string]any{
		"worker_id":     42,
		"queue_wait_ms": 15.5,
		"duration_ms":   100.25,
		"queue_depth":   5,
		"status":        200,
		"error":         "test error",
		"custom_field":  "custom_value",
	}

	event := NewRequestEvent("test.event", req, metadata)

	// Check basic fields
	if event.Type != "test.event" {
		t.Errorf("expected type 'test.event', got %q", event.Type)
	}
	if event.Method != "GET" {
		t.Errorf("expected method 'GET', got %q", event.Method)
	}
	if event.Path != "/test" {
		t.Errorf("expected path '/test', got %q", event.Path)
	}
	if event.Host != "example.com" {
		t.Errorf("expected host 'example.com', got %q", event.Host)
	}

	// Check extracted fields from metadata
	if event.WorkerID != 42 {
		t.Errorf("expected worker_id 42, got %d", event.WorkerID)
	}
	if event.QueueWaitMs != 15.5 {
		t.Errorf("expected queue_wait_ms 15.5, got %f", event.QueueWaitMs)
	}
	if event.DurationMs != 100.25 {
		t.Errorf("expected duration_ms 100.25, got %f", event.DurationMs)
	}
	if event.QueueDepth != 5 {
		t.Errorf("expected queue_depth 5, got %d", event.QueueDepth)
	}
	if event.Status != 200 {
		t.Errorf("expected status 200, got %d", event.Status)
	}
	if event.Error != "test error" {
		t.Errorf("expected error 'test error', got %q", event.Error)
	}

	// Check custom metadata is preserved
	if event.Metadata["custom_field"] != "custom_value" {
		t.Errorf("expected custom_field 'custom_value', got %v", event.Metadata["custom_field"])
	}

	// Check timestamp is recent
	if time.Since(event.Timestamp) > time.Second {
		t.Error("timestamp is not recent")
	}
}

func TestNewRequestEvent_EmptyMetadata(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/test", nil)

	event := NewRequestEvent("empty.test", req, nil)

	if event.Type != "empty.test" {
		t.Errorf("expected type 'empty.test', got %q", event.Type)
	}
	if event.WorkerID != -1 {
		t.Errorf("expected default worker_id -1, got %d", event.WorkerID)
	}
	if event.Metadata == nil {
		t.Error("metadata should not be nil")
	}
	if len(event.Metadata) != 0 {
		t.Errorf("expected empty metadata, got %d entries", len(event.Metadata))
	}
}

func TestNewSystemEvent(t *testing.T) {
	metadata := map[string]any{
		"graceful": true,
		"reason":   "shutdown",
	}

	event := NewSystemEvent("system.shutdown", metadata)

	if event.Type != "system.shutdown" {
		t.Errorf("expected type 'system.shutdown', got %q", event.Type)
	}
	if event.WorkerID != -1 {
		t.Errorf("expected worker_id -1, got %d", event.WorkerID)
	}
	if event.Method != "" {
		t.Errorf("expected empty method, got %q", event.Method)
	}
	if event.Path != "" {
		t.Errorf("expected empty path, got %q", event.Path)
	}

	// Check metadata is copied
	if event.Metadata["graceful"] != true {
		t.Errorf("expected graceful true, got %v", event.Metadata["graceful"])
	}
	if event.Metadata["reason"] != "shutdown" {
		t.Errorf("expected reason 'shutdown', got %v", event.Metadata["reason"])
	}

	// Check timestamp is recent
	if time.Since(event.Timestamp) > time.Second {
		t.Error("timestamp is not recent")
	}
}

func TestEvent_ImmutableMetadata(t *testing.T) {
	metadata := map[string]any{
		"key": "original",
	}

	req := httptest.NewRequest("GET", "/", nil)
	event := NewRequestEvent("test", req, metadata)

	// Modify original metadata
	metadata["key"] = "modified"
	metadata["new_key"] = "new_value"

	// Event should have copy, not reference
	if event.Metadata["key"] != "original" {
		t.Error("event metadata was modified after creation")
	}
	if _, exists := event.Metadata["new_key"]; exists {
		t.Error("event metadata was modified after creation")
	}
}
