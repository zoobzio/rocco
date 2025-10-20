package rocco

import (
	"fmt"
	"strings"

	"github.com/zoobzio/sentinel"
)

// metadataToSchema converts sentinel ModelMetadata to OpenAPI Schema
func metadataToSchema(meta sentinel.ModelMetadata) *Schema {
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	var required []string

	for _, field := range meta.Fields {
		// Parse json tag to get property name and omitempty
		propName, isRequired := parseJSONTag(field)
		if propName == "-" {
			// Skip fields with json:"-"
			continue
		}

		// Convert field type to schema
		fieldSchema := goTypeToSchema(field.Type)

		schema.Properties[propName] = fieldSchema

		if isRequired {
			required = append(required, propName)
		}
	}

	if len(required) > 0 {
		schema.Required = required
	}

	return schema
}

// parseJSONTag extracts the JSON property name and determines if field is required
func parseJSONTag(field sentinel.FieldMetadata) (name string, required bool) {
	jsonTag, exists := field.Tags["json"]
	if !exists {
		// No json tag - use field name lowercased
		return strings.ToLower(field.Name), true
	}

	parts := strings.Split(jsonTag, ",")
	name = parts[0]

	if name == "" {
		// Empty name means use field name
		name = strings.ToLower(field.Name)
	}

	// Check for omitempty
	required = true
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "omitempty" {
			required = false
			break
		}
	}

	return name, required
}

// goTypeToSchema converts a Go type string to an OpenAPI Schema
func goTypeToSchema(goType string) *Schema {
	// Handle pointers
	goType = strings.TrimPrefix(goType, "*")

	// Handle arrays/slices
	if strings.HasPrefix(goType, "[]") {
		elementType := strings.TrimPrefix(goType, "[]")
		return &Schema{
			Type:  "array",
			Items: goTypeToSchema(elementType),
		}
	}

	// Handle maps
	if strings.HasPrefix(goType, "map[") {
		return &Schema{
			Type:                 "object",
			AdditionalProperties: true,
		}
	}

	// Basic type mapping
	switch goType {
	case "string":
		return &Schema{Type: "string"}
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return &Schema{Type: "integer"}
	case "float32", "float64":
		return &Schema{Type: "number"}
	case "bool":
		return &Schema{Type: "boolean"}
	case "time.Time":
		return &Schema{Type: "string", Format: "date-time"}
	default:
		// Complex type - reference to component schema
		// Extract just the type name (remove package prefix)
		typeName := goType
		if idx := strings.LastIndex(goType, "."); idx != -1 {
			typeName = goType[idx+1:]
		}
		return &Schema{Ref: "#/components/schemas/" + typeName}
	}
}

// schemaName extracts a clean schema name from ModelMetadata
func schemaName(meta sentinel.ModelMetadata) string {
	// Use TypeName which is already the clean struct name
	return meta.TypeName
}

// statusCodeToResponseName maps HTTP status codes to OpenAPI response component names
func statusCodeToResponseName(code int) string {
	switch code {
	case 400:
		return "BadRequest"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "NotFound"
	case 409:
		return "Conflict"
	case 422:
		return "UnprocessableEntity"
	case 429:
		return "TooManyRequests"
	case 500:
		return "InternalServerError"
	default:
		return "InternalServerError"
	}
}

// isNoBodySchema checks if a schema represents the NoBody type
func isNoBodySchema(schema *Schema) bool {
	// NoBody will have TypeName "NoBody" in sentinel metadata
	// The schema will be an empty object with no properties
	return schema != nil && schema.Type == "object" && len(schema.Properties) == 0
}

// setOperationForMethod sets the operation on the correct method field of PathItem
func setOperationForMethod(pathItem *PathItem, method string, operation *Operation) {
	switch method {
	case "GET":
		pathItem.Get = operation
	case "POST":
		pathItem.Post = operation
	case "PUT":
		pathItem.Put = operation
	case "DELETE":
		pathItem.Delete = operation
	case "PATCH":
		pathItem.Patch = operation
	case "OPTIONS":
		pathItem.Options = operation
	case "HEAD":
		pathItem.Head = operation
	}
}

// GenerateOpenAPI creates an OpenAPI specification from registered handlers
func (e *Engine) GenerateOpenAPI(info Info) *OpenAPI {
	spec := &OpenAPI{
		OpenAPI: "3.0.3",
		Info:    info,
		Paths:   make(map[string]PathItem),
		Components: &Components{
			Schemas:   make(map[string]*Schema),
			Responses: make(map[string]*Response),
		},
	}

	// Add standard error responses to components
	spec.Components.Responses["BadRequest"] = &Response{
		Description: "Bad Request",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{
					Type: "object",
					Properties: map[string]*Schema{
						"error": {Type: "string"},
					},
					Required: []string{"error"},
				},
			},
		},
	}
	spec.Components.Responses["Unauthorized"] = &Response{
		Description: "Unauthorized",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}
	spec.Components.Responses["Forbidden"] = &Response{
		Description: "Forbidden",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}
	spec.Components.Responses["NotFound"] = &Response{
		Description: "Not Found",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}
	spec.Components.Responses["Conflict"] = &Response{
		Description: "Conflict",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}
	spec.Components.Responses["UnprocessableEntity"] = &Response{
		Description: "Unprocessable Entity",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}
	spec.Components.Responses["TooManyRequests"] = &Response{
		Description: "Too Many Requests",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}
	spec.Components.Responses["InternalServerError"] = &Response{
		Description: "Internal Server Error",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
			},
		},
	}

	// Add ErrorResponse schema
	spec.Components.Schemas["ErrorResponse"] = &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"error": {Type: "string"},
		},
		Required: []string{"error"},
	}

	// Track unique schemas to add to components
	schemas := make(map[string]*Schema)
	processedTypes := make(map[string]bool) // Prevent infinite recursion

	// Helper to recursively collect schemas for a type and its relationships
	var collectSchemas func(meta sentinel.ModelMetadata)
	collectSchemas = func(meta sentinel.ModelMetadata) {
		typeName := meta.TypeName
		if processedTypes[typeName] {
			return
		}
		processedTypes[typeName] = true

		// Add this type's schema
		schemas[typeName] = metadataToSchema(meta)

		// Process all related types
		for _, rel := range meta.Relationships {
			// Lookup related type metadata
			if relMeta, found := sentinel.Lookup(rel.To); found {
				collectSchemas(relMeta)
			}
		}
	}

	// Iterate over registered handlers
	for _, handler := range e.handlers {
		path := handler.Path()
		method := handler.Method()

		// Get or create PathItem
		pathItem, exists := spec.Paths[path]
		if !exists {
			pathItem = PathItem{}
		}

		// Build operation
		operation := &Operation{
			OperationID: handler.Name(),
			Summary:     handler.Summary(),
			Description: handler.Description(),
			Tags:        handler.Tags(),
			Responses:   make(map[string]Response),
		}

		// Add path parameters
		for _, paramName := range handler.PathParams() {
			operation.Parameters = append(operation.Parameters, Parameter{
				Name:     paramName,
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: "string"},
			})
		}

		// Add query parameters
		for _, paramName := range handler.QueryParams() {
			operation.Parameters = append(operation.Parameters, Parameter{
				Name:     paramName,
				In:       "query",
				Required: false,
				Schema:   &Schema{Type: "string"},
			})
		}

		// Add request body if not NoBody
		inputSchema := handler.InputSchema()
		if !isNoBodySchema(inputSchema) {
			inputSchemaName := handler.InputTypeName()
			// Recursively collect input type and all nested types
			if inputMeta, found := sentinel.Lookup(inputSchemaName); found {
				collectSchemas(inputMeta)
			}

			operation.RequestBody = &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/" + inputSchemaName},
					},
				},
			}
		}

		// Add success response
		outputSchemaName := handler.OutputTypeName()
		// Recursively collect output type and all nested types
		if outputMeta, found := sentinel.Lookup(outputSchemaName); found {
			collectSchemas(outputMeta)
		}

		successStatus := handler.SuccessStatus()
		operation.Responses[fmt.Sprintf("%d", successStatus)] = Response{
			Description: "Success",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Ref: "#/components/schemas/" + outputSchemaName},
				},
			},
		}

		// Add error responses
		for _, errorCode := range handler.ErrorCodes() {
			responseName := statusCodeToResponseName(errorCode)
			operation.Responses[fmt.Sprintf("%d", errorCode)] = Response{
				Description: responseName,
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"},
					},
				},
			}
		}

		// Set operation on path item
		setOperationForMethod(&pathItem, method, operation)

		// Update paths
		spec.Paths[path] = pathItem
	}

	// Add collected schemas to components
	for name, schema := range schemas {
		spec.Components.Schemas[name] = schema
	}

	return spec
}
