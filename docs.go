package rocco

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zoobzio/sentinel"
)

func init() {
	// Register tags with sentinel for extraction
	// validate: runtime validation that also drives OpenAPI constraints
	sentinel.Tag("validate")
	// Documentation-only tags
	sentinel.Tag("example")
	sentinel.Tag("description")
}

// parseFloat64 parses a string to *float64
func parseFloat64(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

// parseInt parses a string to *int
func parseInt(s string) *int {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

// parseBool parses a string to *bool
func parseBool(s string) *bool {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return nil
	}
	return &v
}

// parseExample parses an example value based on the schema type
func parseExample(value string, schemaType string) any {
	if value == "" {
		return nil
	}

	switch schemaType {
	case "integer":
		if v, err := strconv.Atoi(value); err == nil {
			return v
		}
	case "number":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return v
		}
	case "boolean":
		if v, err := strconv.ParseBool(value); err == nil {
			return v
		}
	case "array":
		// For arrays, split by comma
		parts := strings.Split(value, ",")
		result := make([]any, len(parts))
		for i, part := range parts {
			result[i] = strings.TrimSpace(part)
		}
		return result
	}

	// Default to string
	return value
}

// parseEnum parses comma-separated enum values based on schema type
func parseEnum(value string, schemaType string) []any {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]any, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch schemaType {
		case "integer":
			if v, err := strconv.Atoi(part); err == nil {
				result = append(result, v)
			}
		case "number":
			if v, err := strconv.ParseFloat(part, 64); err == nil {
				result = append(result, v)
			}
		case "boolean":
			if v, err := strconv.ParseBool(part); err == nil {
				result = append(result, v)
			}
		default:
			result = append(result, part)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// parseValidateTag parses go-playground/validator tag and extracts OpenAPI constraints
func parseValidateTag(validateTag string, goType string) map[string]any {
	if validateTag == "" {
		return nil
	}

	constraints := make(map[string]any)
	rules := strings.Split(validateTag, ",")

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}

		// Split on = for parameterized rules
		parts := strings.SplitN(rule, "=", 2)
		tag := parts[0]
		var param string
		if len(parts) > 1 {
			param = parts[1]
		}

		// Determine base type (without pointer/array prefix)
		baseType := strings.TrimPrefix(goType, "*")
		baseType = strings.TrimPrefix(baseType, "[]")
		isArray := strings.HasPrefix(strings.TrimPrefix(goType, "*"), "[]")
		isNumeric := baseType == "int" || baseType == "int8" || baseType == "int16" ||
			baseType == "int32" || baseType == "int64" || baseType == "uint" ||
			baseType == "uint8" || baseType == "uint16" || baseType == "uint32" ||
			baseType == "uint64" || baseType == "float32" || baseType == "float64"
		isString := baseType == "string"

		switch tag {
		// Numeric constraints
		case "min":
			if isNumeric {
				constraints["minimum"] = parseFloat64(param)
			} else if isString {
				constraints["minLength"] = parseInt(param)
			}
			// Note: min/max on arrays applies to elements with dive, not array length
		case "max":
			if isNumeric {
				constraints["maximum"] = parseFloat64(param)
			} else if isString {
				constraints["maxLength"] = parseInt(param)
			}
			// Note: min/max on arrays applies to elements with dive, not array length

		// Array length constraints (validator uses len, min_items, max_items, or dive)
		case "len":
			if isArray {
				// len=N means exactly N items
				constraints["minItems"] = parseInt(param)
				constraints["maxItems"] = parseInt(param)
			} else if isString {
				// len=N means exactly N characters
				constraints["minLength"] = parseInt(param)
				constraints["maxLength"] = parseInt(param)
			}
		case "gte":
			if isNumeric {
				constraints["minimum"] = parseFloat64(param)
			}
		case "lte":
			if isNumeric {
				constraints["maximum"] = parseFloat64(param)
			}
		case "gt":
			if isNumeric {
				constraints["minimum"] = parseFloat64(param)
				constraints["exclusiveMinimum"] = parseBool("true")
			}
		case "lt":
			if isNumeric {
				constraints["maximum"] = parseFloat64(param)
				constraints["exclusiveMaximum"] = parseBool("true")
			}

		// String format validations
		case "email":
			constraints["format"] = "email"
		case "url":
			constraints["format"] = "uri"
		case "uuid", "uuid4", "uuid5":
			constraints["format"] = "uuid"
		case "datetime":
			constraints["format"] = "date-time"
		case "ipv4":
			constraints["format"] = "ipv4"
		case "ipv6":
			constraints["format"] = "ipv6"

		// Array validations
		case "unique":
			if isArray {
				constraints["uniqueItems"] = parseBool("true")
			}

		// Enum (oneof)
		case "oneof":
			if param != "" {
				// oneof uses space-separated values
				values := strings.Split(param, " ")
				enumValues := make([]any, 0, len(values))
				for _, v := range values {
					v = strings.TrimSpace(v)
					if v == "" {
						continue
					}
					// Parse based on type
					if isNumeric {
						if baseType == "float32" || baseType == "float64" {
							if fv, err := strconv.ParseFloat(v, 64); err == nil {
								enumValues = append(enumValues, fv)
							}
						} else {
							if iv, err := strconv.Atoi(v); err == nil {
								enumValues = append(enumValues, iv)
							}
						}
					} else {
						enumValues = append(enumValues, v)
					}
				}
				if len(enumValues) > 0 {
					constraints["enum"] = enumValues
				}
			}

		// Required is handled via json tag omitempty, skip here
		case "required":
			// No-op: required is determined by json tag

		// Pattern matching
		case "contains", "startswith", "endswith":
			// These could be mapped to pattern if we construct regex
			// For now, skip as they're not direct OpenAPI mappings
		}
	}

	return constraints
}

// applyOpenAPITags extracts OpenAPI tags from field metadata and applies them to the schema
func applyOpenAPITags(schema *Schema, field sentinel.FieldMetadata) {
	// First, parse validate tag to extract constraints
	if validateTag := field.Tags["validate"]; validateTag != "" {
		constraints := parseValidateTag(validateTag, field.Type)
		for key, value := range constraints {
			switch key {
			case "minimum":
				if v, ok := value.(*float64); ok {
					schema.Minimum = v
				}
			case "maximum":
				if v, ok := value.(*float64); ok {
					schema.Maximum = v
				}
			case "exclusiveMinimum":
				if v, ok := value.(*bool); ok {
					schema.ExclusiveMinimum = v
				}
			case "exclusiveMaximum":
				if v, ok := value.(*bool); ok {
					schema.ExclusiveMaximum = v
				}
			case "minLength":
				if v, ok := value.(*int); ok {
					schema.MinLength = v
				}
			case "maxLength":
				if v, ok := value.(*int); ok {
					schema.MaxLength = v
				}
			case "minItems":
				if v, ok := value.(*int); ok {
					schema.MinItems = v
				}
			case "maxItems":
				if v, ok := value.(*int); ok {
					schema.MaxItems = v
				}
			case "uniqueItems":
				if v, ok := value.(*bool); ok {
					schema.UniqueItems = v
				}
			case "format":
				if v, ok := value.(string); ok {
					schema.Format = v
				}
			case "enum":
				if v, ok := value.([]any); ok {
					schema.Enum = v
				}
			}
		}
	}

	// Then, apply documentation-only tags (can override validate-derived values)
	if desc := field.Tags["description"]; desc != "" {
		schema.Description = desc
	}

	if example := field.Tags["example"]; example != "" {
		schema.Example = parseExample(example, schema.Type)
	}
}

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

		// Apply OpenAPI tags to field schema
		applyOpenAPITags(fieldSchema, field)

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
