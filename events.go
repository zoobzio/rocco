package rocco

import "github.com/zoobzio/capitan"

// Engine lifecycle signals.
const (
	// EngineCreated is emitted when an Engine instance is created.
	// Fields: HostKey, PortKey.
	EngineCreated capitan.Signal = "http.engine.created"

	// EngineStarting is emitted when the server starts listening for requests.
	// Fields: HostKey, PortKey, AddressKey.
	EngineStarting capitan.Signal = "http.engine.starting"

	// EngineShutdownStarted is emitted when graceful shutdown is initiated.
	// Fields: none.
	EngineShutdownStarted capitan.Signal = "http.engine.shutdown.started"

	// EngineShutdownComplete is emitted when shutdown finishes.
	// Fields: GracefulKey, ErrorKey (if failed).
	EngineShutdownComplete capitan.Signal = "http.engine.shutdown.complete"
)

// Handler registration signals.
const (
	// HandlerRegistered is emitted when a handler is registered with the engine.
	// Fields: HandlerNameKey, MethodKey, PathKey.
	HandlerRegistered capitan.Signal = "http.handler.registered"
)

// Request lifecycle signals.
const (
	// RequestReceived is emitted when a request is received.
	// Fields: MethodKey, PathKey, HandlerNameKey.
	RequestReceived capitan.Signal = "http.request.received"

	// RequestCompleted is emitted when a request completes successfully.
	// Fields: MethodKey, PathKey, HandlerNameKey, StatusCodeKey, DurationMsKey.
	RequestCompleted capitan.Signal = "http.request.completed"

	// RequestFailed is emitted when a request fails with an error.
	// Fields: MethodKey, PathKey, HandlerNameKey, StatusCodeKey, DurationMsKey, ErrorKey.
	RequestFailed capitan.Signal = "http.request.failed"
)

// Handler processing signals.
const (
	// HandlerExecuting is emitted when handler execution begins.
	// Fields: HandlerNameKey.
	HandlerExecuting capitan.Signal = "http.handler.executing"

	// HandlerSuccess is emitted when a handler returns successfully.
	// Fields: HandlerNameKey, StatusCodeKey.
	HandlerSuccess capitan.Signal = "http.handler.success"

	// HandlerError is emitted when a handler returns an error.
	// Fields: HandlerNameKey, ErrorKey.
	HandlerError capitan.Signal = "http.handler.error"

	// HandlerSentinelError is emitted when a declared sentinel error is returned.
	// Fields: HandlerNameKey, ErrorKey, StatusCodeKey.
	HandlerSentinelError capitan.Signal = "http.handler.sentinel.error"

	// HandlerUndeclaredSentinel is emitted when an undeclared sentinel error is returned (programming error).
	// Fields: HandlerNameKey, ErrorKey, StatusCodeKey.
	HandlerUndeclaredSentinel capitan.Signal = "http.handler.sentinel.undeclared"

	// RequestParamsInvalid is emitted when path or query parameter extraction fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestParamsInvalid capitan.Signal = "http.request.params.invalid"

	// RequestBodyReadError is emitted when reading the request body fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestBodyReadError capitan.Signal = "http.request.body.read.error"

	// RequestBodyParseError is emitted when parsing the JSON request body fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestBodyParseError capitan.Signal = "http.request.body.parse.error"

	// RequestValidationInputFailed is emitted when input validation fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestValidationInputFailed capitan.Signal = "http.request.validation.input.failed"

	// RequestValidationOutputFailed is emitted when output validation fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestValidationOutputFailed capitan.Signal = "http.request.validation.output.failed"

	// RequestResponseMarshalError is emitted when marshaling the response fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestResponseMarshalError capitan.Signal = "http.request.response.marshal.error"
)

// Event field keys (primitive types only).
var (
	// Engine fields.
	HostKey    = capitan.NewStringKey("host")
	PortKey    = capitan.NewIntKey("port")
	AddressKey = capitan.NewStringKey("address")

	// Request/Response fields.
	MethodKey      = capitan.NewStringKey("method")
	PathKey        = capitan.NewStringKey("path")
	HandlerNameKey = capitan.NewStringKey("handler_name")
	StatusCodeKey  = capitan.NewIntKey("status_code")
	DurationMsKey  = capitan.NewInt64Key("duration_ms")
	ErrorKey       = capitan.NewStringKey("error")
	GracefulKey    = capitan.NewBoolKey("graceful")
)
