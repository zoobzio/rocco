package rocco

import "github.com/zoobzio/openapi"

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
	PathParams     []string `json:"pathParams,omitempty" yaml:"pathParams,omitempty"`
	QueryParams    []string `json:"queryParams,omitempty" yaml:"queryParams,omitempty"`
	InputTypeName  string   `json:"inputTypeName" yaml:"inputTypeName"`
	OutputTypeName string   `json:"outputTypeName" yaml:"outputTypeName"`
	SuccessStatus  int      `json:"successStatus" yaml:"successStatus"`
	ErrorCodes     []int    `json:"errorCodes,omitempty" yaml:"errorCodes,omitempty"`

	// Authentication & Authorization
	RequiresAuth bool       `json:"requiresAuth" yaml:"requiresAuth"`
	ScopeGroups  [][]string `json:"scopeGroups,omitempty" yaml:"scopeGroups,omitempty"` // OR within group, AND across groups
	RoleGroups   [][]string `json:"roleGroups,omitempty" yaml:"roleGroups,omitempty"`   // OR within group, AND across groups

	// Rate Limiting
	UsageLimits []UsageLimit `json:"usageLimits,omitempty" yaml:"usageLimits,omitempty"`

	// Streaming
	IsStream bool `json:"isStream,omitempty" yaml:"isStream,omitempty"` // SSE stream handler
}

// EngineSpec contains declarative configuration for the API engine.
// This spec is serializable and represents API-level metadata
// used for OpenAPI generation and documentation.
type EngineSpec struct {
	// OpenAPI Info
	Info openapi.Info `json:"info" yaml:"info"`

	// Global Tags with descriptions
	Tags []openapi.Tag `json:"tags,omitempty" yaml:"tags,omitempty"`

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
