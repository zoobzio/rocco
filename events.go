package rocco

import "github.com/zoobzio/capitan"

// Engine lifecycle signals.
var (
	// EngineCreated is emitted when an Engine instance is created.
	// Fields: HostKey, PortKey.
	EngineCreated = capitan.NewSignal("http.engine.created", "HTTP engine instance created with configured host and port")

	// EngineStarting is emitted when the server starts listening for requests.
	// Fields: HostKey, PortKey, AddressKey.
	EngineStarting = capitan.NewSignal("http.engine.starting", "HTTP server starting to listen for requests on configured address")

	// EngineShutdownStarted is emitted when graceful shutdown is initiated.
	// Fields: none.
	EngineShutdownStarted = capitan.NewSignal("http.engine.shutdown.started", "HTTP engine graceful shutdown initiated")

	// EngineShutdownComplete is emitted when shutdown finishes.
	// Fields: GracefulKey, ErrorKey (if failed).
	EngineShutdownComplete = capitan.NewSignal("http.engine.shutdown.complete", "HTTP engine shutdown completed, graceful or with error")
)

// Handler registration signals.
var (
	// HandlerRegistered is emitted when a handler is registered with the engine.
	// Fields: HandlerNameKey, MethodKey, PathKey.
	HandlerRegistered = capitan.NewSignal("http.handler.registered", "HTTP handler registered with engine for specific route")
)

// Request lifecycle signals.
var (
	// RequestReceived is emitted when a request is received.
	// Fields: MethodKey, PathKey, HandlerNameKey.
	RequestReceived = capitan.NewSignal("http.request.received", "HTTP request received by engine and routed to handler")

	// RequestCompleted is emitted when a request completes successfully.
	// Fields: MethodKey, PathKey, HandlerNameKey, StatusCodeKey, DurationMsKey.
	RequestCompleted = capitan.NewSignal("http.request.completed", "HTTP request completed successfully with response sent")

	// RequestFailed is emitted when a request fails with an error.
	// Fields: MethodKey, PathKey, HandlerNameKey, StatusCodeKey, DurationMsKey, ErrorKey.
	RequestFailed = capitan.NewSignal("http.request.failed", "HTTP request failed during processing with error")
)

// Handler processing signals.
var (
	// HandlerExecuting is emitted when handler execution begins.
	// Fields: HandlerNameKey.
	HandlerExecuting = capitan.NewSignal("http.handler.executing", "Handler execution started for incoming request")

	// HandlerSuccess is emitted when a handler returns successfully.
	// Fields: HandlerNameKey, StatusCodeKey.
	HandlerSuccess = capitan.NewSignal("http.handler.success", "Handler completed successfully and returned response")

	// HandlerError is emitted when a handler returns an error.
	// Fields: HandlerNameKey, ErrorKey.
	HandlerError = capitan.NewSignal("http.handler.error", "Handler returned unexpected error during execution")

	// HandlerSentinelError is emitted when a declared sentinel error is returned.
	// Fields: HandlerNameKey, ErrorKey, StatusCodeKey.
	HandlerSentinelError = capitan.NewSignal("http.handler.sentinel.error", "Handler returned declared sentinel error mapped to HTTP status")

	// HandlerUndeclaredSentinel is emitted when an undeclared sentinel error is returned (programming error).
	// Fields: HandlerNameKey, ErrorKey, StatusCodeKey.
	HandlerUndeclaredSentinel = capitan.NewSignal("http.handler.sentinel.undeclared", "Handler returned undeclared sentinel error, programming error detected")

	// RequestParamsInvalid is emitted when path or query parameter extraction fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestParamsInvalid = capitan.NewSignal("http.request.params.invalid", "Request path or query parameter extraction failed")

	// RequestBodyReadError is emitted when reading the request body fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestBodyReadError = capitan.NewSignal("http.request.body.read.error", "Failed to read request body from HTTP stream")

	// RequestBodyParseError is emitted when parsing the JSON request body fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestBodyParseError = capitan.NewSignal("http.request.body.parse.error", "Failed to parse JSON request body")

	// RequestValidationInputFailed is emitted when input validation fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestValidationInputFailed = capitan.NewSignal("http.request.validation.input.failed", "Request input validation failed against defined rules")

	// RequestValidationOutputFailed is emitted when output validation fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestValidationOutputFailed = capitan.NewSignal("http.request.validation.output.failed", "Response output validation failed, internal error")

	// RequestResponseMarshalError is emitted when marshaling the response fails.
	// Fields: HandlerNameKey, ErrorKey.
	RequestResponseMarshalError = capitan.NewSignal("http.request.response.marshal.error", "Failed to marshal response to JSON")
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
