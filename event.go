package rocco

import (
	"net/http"
	"time"
)

// Event represents a snapshot of system state at a specific moment.
// Events are immutable once created and safe for async processing.
// They capture relevant data from Jobs and add temporal context for
// linear analysis in observability systems.
type Event struct {
	// Event identification
	Type      string    // Event type (e.g., "request.received", "worker.started")
	Timestamp time.Time // When this event occurred

	// Request context (immutable snapshot)
	Method string // HTTP method
	Path   string // Request path
	Host   string // Request host

	// Processing state (snapshot at event time)
	Status   int            // HTTP status code
	Error    string         // Error message if any
	Metadata map[string]any // Event-specific metadata

	// System context
	WorkerID    int     // Worker processing this request (-1 if N/A)
	QueueDepth  int     // Queue depth at event time
	QueueWaitMs float64 // Milliseconds spent in queue
	DurationMs  float64 // Processing duration in milliseconds
}

// NewRequestEvent creates an Event from an HTTP request and additional context.
// It copies relevant data to ensure immutability.
func NewRequestEvent(eventType string, r *http.Request, metadata map[string]any) *Event {
	e := &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Host:      r.Host,
		WorkerID:  -1, // Default for non-worker events
	}

	// Copy provided metadata
	e.Metadata = make(map[string]any)
	for k, v := range metadata {
		e.Metadata[k] = v
	}

	// Extract common fields from metadata if present
	if workerID, ok := e.Metadata["worker_id"].(int); ok {
		e.WorkerID = workerID
	}
	if queueWait, ok := e.Metadata["queue_wait_ms"].(float64); ok {
		e.QueueWaitMs = queueWait
	}
	if duration, ok := e.Metadata["duration_ms"].(float64); ok {
		e.DurationMs = duration
	}
	if queueDepth, ok := e.Metadata["queue_depth"].(int); ok {
		e.QueueDepth = queueDepth
	}
	if status, ok := e.Metadata["status"].(int); ok {
		e.Status = status
	}
	if errMsg, ok := e.Metadata["error"].(string); ok {
		e.Error = errMsg
	}

	return e
}

// NewSystemEvent creates an Event for system-level events without a request.
// Used for shutdown events and other engine-level events.
func NewSystemEvent(eventType string, metadata map[string]any) *Event {
	e := &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		WorkerID:  -1,
	}

	// Copy metadata
	e.Metadata = make(map[string]any)
	for k, v := range metadata {
		e.Metadata[k] = v
	}

	return e
}
