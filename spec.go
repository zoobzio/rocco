package rocco

import "github.com/zoobz-io/openapi"

// BodyKind enumerates how a request or response body is produced or consumed.
type BodyKind string

const (
	// BodyEncoded is a codec-encoded typed value (the default).
	BodyEncoded BodyKind = "encoded"
	// BodyNone means no body is read or written.
	BodyNone BodyKind = "none"
	// BodyStream is a Server-Sent Events stream.
	BodyStream BodyKind = "stream"
)

// RequestContract declares how a handler consumes the request body.
// It is filled by the handler constructors from the input type parameter;
// users do not set it directly.
type RequestContract struct {
	Kind BodyKind `json:"kind" yaml:"kind"`
	// MaxBytes is the maximum request body size (0 = unlimited).
	MaxBytes int64 `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
	// MediaTypes lists the content types the handler accepts for the body.
	// For encoded bodies this is the codec's content type.
	MediaTypes []string `json:"mediaTypes,omitempty" yaml:"mediaTypes,omitempty"`
}

// ResponseContract declares the success response shape of a handler.
// It is filled by the handler constructors from the output type parameter;
// users refine it through builder methods such as WithSuccessStatus.
// Both the runtime write path and OpenAPI generation read this contract,
// so they cannot disagree.
type ResponseContract struct {
	Kind   BodyKind `json:"kind" yaml:"kind"`
	Status int      `json:"status" yaml:"status"`
	// Redirect marks a BodyNone response that carries a Location header.
	Redirect bool `json:"redirect,omitempty" yaml:"redirect,omitempty"`
	// MediaTypes lists the content types the response body may carry.
	// For encoded bodies this is the codec's content type; for streams it is
	// text/event-stream. The runtime Content-Type header and the OpenAPI
	// content keys both come from this list.
	MediaTypes []string `json:"mediaTypes,omitempty" yaml:"mediaTypes,omitempty"`
}

// primaryMediaType returns the first declared media type, or JSON when the
// list is empty — the encoded default.
func primaryMediaType(mediaTypes []string) string {
	if len(mediaTypes) > 0 {
		return mediaTypes[0]
	}
	return ContentTypeJSON
}

// HandlerSpec contains declarative configuration for a route handler.
// This spec is serializable and represents all metadata about a handler
// that can be used for documentation, authorization checks, and filtering.
type HandlerSpec struct {
	// Routing
	Name   string `json:"name" yaml:"name"`
	Method string `json:"method" yaml:"method"`
	Path   string `json:"path" yaml:"path"`

	// Documentation
	Summary     string   `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Request/Response
	PathParams     []string         `json:"pathParams,omitempty" yaml:"pathParams,omitempty"`
	QueryParams    []string         `json:"queryParams,omitempty" yaml:"queryParams,omitempty"`
	InputTypeFQDN  string           `json:"-" yaml:"-"`
	InputTypeName  string           `json:"inputTypeName" yaml:"inputTypeName"`
	OutputTypeFQDN string           `json:"-" yaml:"-"`
	OutputTypeName string           `json:"outputTypeName" yaml:"outputTypeName"`
	Request        RequestContract  `json:"request" yaml:"request"`
	Response       ResponseContract `json:"response" yaml:"response"`

	// Authentication & Authorization
	RequiresAuth bool       `json:"requiresAuth" yaml:"requiresAuth"`
	ScopeGroups  [][]string `json:"scopeGroups,omitempty" yaml:"scopeGroups,omitempty"` // OR within group, AND across groups
	RoleGroups   [][]string `json:"roleGroups,omitempty" yaml:"roleGroups,omitempty"`   // OR within group, AND across groups

	// Rate Limiting
	UsageLimits []UsageLimit `json:"usageLimits,omitempty" yaml:"usageLimits,omitempty"`
}

// EngineSpec contains declarative configuration for the API engine.
// This spec is serializable and represents API-level metadata
// used for OpenAPI generation and documentation.
type EngineSpec struct {
	// OpenAPI Info
	Info openapi.Info `json:"info" yaml:"info"`

	// Global Tags with descriptions
	Tags []openapi.Tag `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Tag Groups for hierarchical tag organization (x-tagGroups vendor extension)
	TagGroups []openapi.TagGroup `json:"x-tagGroups,omitempty" yaml:"x-tagGroups,omitempty"`

	// Servers
	Servers []openapi.Server `json:"servers,omitempty" yaml:"servers,omitempty"`

	// External Documentation
	ExternalDocs *openapi.ExternalDocumentation `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`

	// Global Security (optional, for APIs that require auth on all endpoints)
	Security []openapi.SecurityRequirement `json:"security,omitempty" yaml:"security,omitempty"`
}

// DefaultEngineSpec returns an EngineSpec with sensible defaults.
func DefaultEngineSpec() *EngineSpec {
	return &EngineSpec{
		Info: openapi.Info{
			Title:   "API",
			Version: "1.0.0",
		},
		Tags:    []openapi.Tag{},
		Servers: []openapi.Server{},
	}
}
