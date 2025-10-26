package rocco

import "github.com/zoobzio/capitan"

// Engine lifecycle signals.
const (
	EventEngineCreated          capitan.Signal = "engine.created"
	EventEngineStarting         capitan.Signal = "engine.starting"
	EventEngineShutdownStarted  capitan.Signal = "engine.shutdown.started"
	EventEngineShutdownComplete capitan.Signal = "engine.shutdown.complete"
)

// Handler registration signals.
const (
	EventHandlerRegistered capitan.Signal = "handler.registered"
)

// Request lifecycle signals.
const (
	EventRequestReceived  capitan.Signal = "request.received"
	EventRequestCompleted capitan.Signal = "request.completed"
	EventRequestFailed    capitan.Signal = "request.failed"
)

// Handler processing signals.
const (
	EventHandlerExecuting              capitan.Signal = "handler.executing"
	EventHandlerSuccess                capitan.Signal = "handler.success"
	EventHandlerError                  capitan.Signal = "handler.error"
	EventHandlerSentinelError          capitan.Signal = "handler.sentinel_error"
	EventHandlerUndeclaredSentinel     capitan.Signal = "handler.error.undeclared_sentinel"
	EventRequestParamsInvalid          capitan.Signal = "request.params.invalid"
	EventRequestBodyReadError          capitan.Signal = "request.body.read_error"
	EventRequestBodyParseError         capitan.Signal = "request.body.parse_error"
	EventRequestValidationInputFailed  capitan.Signal = "request.validation.input_failed"
	EventRequestValidationOutputFailed capitan.Signal = "request.validation.output_failed"
	EventRequestResponseMarshalError   capitan.Signal = "request.response.marshal_error"
)

// Event field keys (primitive types only).
var (
	// Engine fields.
	KeyHost    = capitan.NewStringKey("host")
	KeyPort    = capitan.NewIntKey("port")
	KeyAddress = capitan.NewStringKey("address")

	// Request/Response fields.
	KeyMethod      = capitan.NewStringKey("method")
	KeyPath        = capitan.NewStringKey("path")
	KeyHandlerName = capitan.NewStringKey("handler_name")
	KeyStatusCode  = capitan.NewIntKey("status_code")
	KeyError       = capitan.NewStringKey("error")
	KeyGraceful    = capitan.NewBoolKey("graceful")
)
