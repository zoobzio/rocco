package rocco

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zoobz-io/openapi"
	"github.com/zoobz-io/sentinel"
)

func init() {
	// Register tags with sentinel for extraction
	// validate: runtime validation that also drives OpenAPI constraints
	sentinel.Tag("validate")
	// Documentation-only tags
	sentinel.Tag("example")
	sentinel.Tag("description")
	// Discriminated union tags
	sentinel.Tag("discriminator")
	sentinel.Tag("discriminate")
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
				// OpenAPI 3.1.0: exclusiveMinimum is the actual bound value
				constraints["exclusiveMinimum"] = parseFloat64(param)
			}
		case "lt":
			if isNumeric {
				// OpenAPI 3.1.0: exclusiveMaximum is the actual bound value
				constraints["exclusiveMaximum"] = parseFloat64(param)
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
func applyOpenAPITags(schema *openapi.Schema, field sentinel.FieldMetadata) {
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
				if v, ok := value.(*float64); ok {
					schema.ExclusiveMinimum = v
				}
			case "exclusiveMaximum":
				if v, ok := value.(*float64); ok {
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
		schemaType := ""
		if schema.Type != nil {
			schemaType = schema.Type.String()
		}
		schema.Example = parseExample(example, schemaType)
	}
}

// metadataToSchema converts sentinel Metadata to OpenAPI Schema
func metadataToSchema(meta sentinel.Metadata) *openapi.Schema {
	schema := &openapi.Schema{
		Type:       openapi.NewSchemaType("object"),
		Properties: make(map[string]*openapi.Schema),
	}

	// First pass: collect discriminator mappings.
	// discriminator:"fieldName" declares "I am the discriminator for fieldName"
	// and provides the propertyName via its own json tag.
	discriminators := make(map[string]string) // target field name -> discriminator json name
	for _, field := range meta.Fields {
		if target, ok := field.Tags["discriminator"]; ok && target != "" {
			jsonName, _ := parseJSONTag(field)
			discriminators[target] = jsonName
		}
	}

	var required []string

	for _, field := range meta.Fields {
		// Parse json tag to get property name and omitempty
		propName, isRequired := parseJSONTag(field)
		if propName == "-" {
			// Skip fields with json:"-"
			continue
		}

		var fieldSchema *openapi.Schema

		// Check for discriminate tag — this field is a union
		if discriminateTag, ok := field.Tags["discriminate"]; ok && discriminateTag != "" {
			fieldSchema = buildDiscriminatedUnionSchema(propName, discriminateTag, discriminators)
		} else {
			// Convert field type to schema
			fieldSchema = goTypeToSchema(field.Type)
		}

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

// buildDiscriminatedUnionSchema creates a oneOf schema with discriminator for a union field.
func buildDiscriminatedUnionSchema(propName string, discriminateTag string, discriminators map[string]string) *openapi.Schema {
	typeNames := strings.Split(discriminateTag, ",")
	oneOf := make([]*openapi.Schema, 0, len(typeNames))
	mapping := make(map[string]string, len(typeNames))

	for _, typeName := range typeNames {
		typeName = strings.TrimSpace(typeName)
		if typeName == "" {
			continue
		}
		ref := "#/components/schemas/" + typeName
		oneOf = append(oneOf, &openapi.Schema{Ref: ref})
		mapping[typeName] = ref
	}

	schema := &openapi.Schema{
		OneOf: oneOf,
	}

	// Attach discriminator if a paired discriminator tag targets this field
	if discriminatorPropName, ok := discriminators[propName]; ok {
		schema.Discriminator = &openapi.Discriminator{
			PropertyName: discriminatorPropName,
			Mapping:      mapping,
		}
	}

	return schema
}

// resolveTypeName finds a sentinel Metadata entry by short type name,
// searching all cached FQDNs for a suffix match. If multiple packages
// register types with the same short name, the first match wins —
// short names must be unique across packages for deterministic results.
func resolveTypeName(shortName string) (sentinel.Metadata, bool) {
	suffix := "." + shortName
	for _, fqdn := range sentinel.Browse() {
		if strings.HasSuffix(fqdn, suffix) {
			return sentinel.Lookup(fqdn)
		}
	}
	return sentinel.Metadata{}, false
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
func goTypeToSchema(goType string) *openapi.Schema {
	// Handle pointers
	goType = strings.TrimPrefix(goType, "*")

	// Handle arrays/slices
	if strings.HasPrefix(goType, "[]") {
		elementType := strings.TrimPrefix(goType, "[]")
		return &openapi.Schema{
			Type:  openapi.NewSchemaType("array"),
			Items: goTypeToSchema(elementType),
		}
	}

	// Handle maps
	if strings.HasPrefix(goType, "map[") {
		// Extract value type from map[K]V
		// Find the closing bracket for the key type
		depth := 0
		valueStart := -1
		for i, ch := range goType {
			if ch == '[' {
				depth++
			} else if ch == ']' {
				depth--
				if depth == 0 {
					valueStart = i + 1
					break
				}
			}
		}
		if valueStart > 0 && valueStart < len(goType) {
			return &openapi.Schema{
				Type:                 openapi.NewSchemaType("object"),
				AdditionalProperties: goTypeToSchema(goType[valueStart:]),
			}
		}
		return &openapi.Schema{
			Type:                 openapi.NewSchemaType("object"),
			AdditionalProperties: true,
		}
	}

	// Basic type mapping
	switch goType {
	case "string":
		return &openapi.Schema{Type: openapi.NewSchemaType("string")}
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return &openapi.Schema{Type: openapi.NewSchemaType("integer")}
	case "float32", "float64":
		return &openapi.Schema{Type: openapi.NewSchemaType("number")}
	case "bool":
		return &openapi.Schema{Type: openapi.NewSchemaType("boolean")}
	case "time.Time":
		return &openapi.Schema{Type: openapi.NewSchemaType("string"), Format: "date-time"}
	default:
		// Complex type - reference to component schema
		// Extract just the type name (remove package prefix)
		typeName := goType
		if idx := strings.LastIndex(goType, "."); idx != -1 {
			typeName = goType[idx+1:]
		}
		return &openapi.Schema{Ref: "#/components/schemas/" + typeName}
	}
}

// schemaName extracts a clean schema name from Metadata
func schemaName(meta sentinel.Metadata) string {
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

// errorCodeToSchemaName converts an error code like "NOT_FOUND" to "NotFound"
func errorCodeToSchemaName(code string) string {
	parts := strings.Split(code, "_")
	var result strings.Builder
	for _, part := range parts {
		if part != "" {
			result.WriteString(strings.ToUpper(part[:1]))
			result.WriteString(strings.ToLower(part[1:]))
		}
	}
	return result.String()
}

// setOperationForMethod sets the operation on the correct method field of PathItem
func setOperationForMethod(pathItem *openapi.PathItem, method string, operation *openapi.Operation) {
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

// isHandlerAccessible checks if an identity has access to a handler based on scope/role requirements.
func isHandlerAccessible(handler Endpoint, identity Identity) bool {
	handlerSpec := handler.Spec()

	// If handler doesn't require auth, it's accessible
	if !handlerSpec.RequiresAuth {
		return true
	}

	// Same semantics as the authorization middleware: OR within group, AND across groups.
	_, _, ok := satisfiesRequirements(identity, handlerSpec.ScopeGroups, handlerSpec.RoleGroups)
	return ok
}

// GenerateOpenAPI creates an OpenAPI specification from registered handlers.
// If identity is provided, only handlers accessible to that identity will be included.
func (e *Engine) GenerateOpenAPI(identity Identity) *openapi.OpenAPI {
	spec := &openapi.OpenAPI{
		OpenAPI: "3.1.0",
		Info:    e.spec.Info,
		Tags:    e.spec.Tags,
		Servers:   e.spec.Servers,
		TagGroups: e.spec.TagGroups,
		Paths:   make(map[string]openapi.PathItem),
		Components: &openapi.Components{
			Schemas:   make(map[string]*openapi.Schema),
			Responses: make(map[string]*openapi.Response),
		},
	}

	if e.spec.ExternalDocs != nil {
		spec.ExternalDocs = e.spec.ExternalDocs
	}

	if len(e.spec.Security) > 0 {
		spec.Security = e.spec.Security
	}

	// Check if any handlers require authentication
	hasAuth := false
	for _, handler := range e.handlers {
		if handler.Spec().RequiresAuth {
			hasAuth = true
			break
		}
	}

	// Add bearer token security scheme if any handler requires auth
	if hasAuth {
		spec.Components.SecuritySchemes = make(map[string]*openapi.SecurityScheme)
		spec.Components.SecuritySchemes["bearerAuth"] = &openapi.SecurityScheme{
			Type:        "http",
			Scheme:      "bearer",
			Description: "Bearer token authentication",
		}
	}

	// Collect all unique error definitions from handlers for schema generation
	errorDefs := make(map[string]ErrorDefinition) // keyed by error code
	for _, handler := range e.handlers {
		for _, errDef := range handler.ErrorDefs() {
			errorDefs[errDef.Code()] = errDef
		}
	}

	// Generate typed error response schemas from collected error definitions.
	// The base shape is scanned from the errorResponse struct — the same
	// declaration writeError encodes — then specialized per error code.
	errRespMeta := sentinel.Scan[errorResponse]()
	for code, errDef := range errorDefs {
		detailsMeta := errDef.DetailsMeta()
		schemaName := "Err" + errorCodeToSchemaName(code)

		schema := metadataToSchema(errRespMeta)
		schema.Properties["code"].Const = code

		// Inline details fields directly on the error schema. The scanned
		// "details" field is typed `any`, which has no useful schema — replace
		// it with the concrete details type, or drop it when there is none.
		if detailsMeta.TypeName != "" && detailsMeta.TypeName != "NoDetails" {
			schema.Properties["details"] = metadataToSchema(detailsMeta)
		} else {
			delete(schema.Properties, "details")
		}

		spec.Components.Schemas[schemaName] = schema
	}

	// Track unique schemas to add to components
	schemas := make(map[string]*openapi.Schema)
	processedTypes := make(map[string]bool) // Prevent infinite recursion

	// Helper to recursively collect schemas for a type and its relationships
	var collectSchemas func(meta sentinel.Metadata)
	collectSchemas = func(meta sentinel.Metadata) {
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

		// Discover types referenced by discriminate tags
		for _, field := range meta.Fields {
			if discriminateTag, ok := field.Tags["discriminate"]; ok && discriminateTag != "" {
				for _, name := range strings.Split(discriminateTag, ",") {
					name = strings.TrimSpace(name)
					if name == "" || processedTypes[name] {
						continue
					}
					if relMeta, found := resolveTypeName(name); found {
						collectSchemas(relMeta)
					}
				}
			}
		}
	}

	// Collect nested types referenced by error detail schemas.
	// We don't call collectSchemas on the details type itself (it's inlined
	// on the error schema), but we do need to collect its nested types.
	for _, errDef := range errorDefs {
		detailsMeta := errDef.DetailsMeta()
		if detailsMeta.TypeName != "" && detailsMeta.TypeName != "NoDetails" {
			for _, rel := range detailsMeta.Relationships {
				if relMeta, found := sentinel.Lookup(rel.To); found {
					collectSchemas(relMeta)
				}
			}
		}
	}

	// Collect standalone model schemas
	for _, model := range e.models {
		if model.schema != nil {
			// Direct schema registration (enums, maps, arbitrary schemas)
			schemas[model.name] = model.schema
		} else {
			collectSchemas(model.meta)
		}
	}

	// Iterate over registered handlers
	for _, handler := range e.handlers {
		// Filter handlers based on identity permissions if provided
		if identity != nil && !isHandlerAccessible(handler, identity) {
			continue
		}

		handlerSpec := handler.Spec()

		// Get or create PathItem
		pathItem, exists := spec.Paths[handlerSpec.Path]
		if !exists {
			pathItem = openapi.PathItem{}
		}

		// Build operation
		operation := &openapi.Operation{
			OperationID: handlerSpec.Name,
			Summary:     handlerSpec.Summary,
			Description: handlerSpec.Description,
			Tags:        handlerSpec.Tags,
			Responses:   make(map[string]openapi.Response),
		}

		// Add path parameters
		for _, paramName := range handlerSpec.PathParams {
			operation.Parameters = append(operation.Parameters, openapi.Parameter{
				Name:     paramName,
				In:       "path",
				Required: true,
				Schema:   &openapi.Schema{Type: openapi.NewSchemaType("string")},
			})
		}

		// Add query parameters
		for _, paramName := range handlerSpec.QueryParams {
			operation.Parameters = append(operation.Parameters, openapi.Parameter{
				Name:     paramName,
				In:       "query",
				Required: false,
				Schema:   &openapi.Schema{Type: openapi.NewSchemaType("string")},
			})
		}

		// Add request body according to the declared request contract.
		if handlerSpec.Request.Kind == BodyEncoded {
			// Recursively collect input type and all nested types
			if inputMeta, found := sentinel.Lookup(handlerSpec.InputTypeFQDN); found {
				collectSchemas(inputMeta)
			}

			contentType := handlerSpec.ContentType
			if contentType == "" {
				contentType = ContentTypeJSON
			}
			operation.RequestBody = &openapi.RequestBody{
				Required: true,
				Content: map[string]openapi.MediaType{
					contentType: {
						Schema: &openapi.Schema{Ref: "#/components/schemas/" + handlerSpec.InputTypeName},
					},
				},
			}
		}

		// Add success response according to the declared response contract.
		// This is the same declaration the runtime write path executes, so the
		// spec cannot describe behavior the handler does not have.
		successStatus := fmt.Sprintf("%d", handlerSpec.Response.Status)
		switch handlerSpec.Response.Kind {
		case BodyStream:
			// SSE stream response. Event schemas resolve through the output type.
			if outputMeta, found := sentinel.Lookup(handlerSpec.OutputTypeFQDN); found {
				collectSchemas(outputMeta)
			}
			operation.Responses[successStatus] = openapi.Response{
				Description: "Server-Sent Events stream",
				Content: map[string]openapi.MediaType{
					"text/event-stream": {
						Schema: &openapi.Schema{
							Type:        openapi.NewSchemaType("string"),
							Description: fmt.Sprintf("SSE stream emitting %s events as JSON", handlerSpec.OutputTypeName),
						},
					},
				},
			}
		case BodyNone:
			// No body. The output type is a marker (e.g. Redirect) and must not
			// appear in component schemas.
			resp := openapi.Response{Description: "No content"}
			if handlerSpec.Response.Redirect {
				resp.Description = "Redirect"
				resp.Headers = map[string]*openapi.Header{
					"Location": {
						Description: "Redirect target URL",
						Schema:      &openapi.Schema{Type: openapi.NewSchemaType("string")},
					},
				}
			}
			operation.Responses[successStatus] = resp
		default: // BodyEncoded
			// Recursively collect output type and all nested types
			if outputMeta, found := sentinel.Lookup(handlerSpec.OutputTypeFQDN); found {
				collectSchemas(outputMeta)
			}
			responseContentType := handlerSpec.ContentType
			if responseContentType == "" {
				responseContentType = ContentTypeJSON
			}
			operation.Responses[successStatus] = openapi.Response{
				Description: "Success",
				Content: map[string]openapi.MediaType{
					responseContentType: {
						Schema: &openapi.Schema{Ref: "#/components/schemas/" + handlerSpec.OutputTypeName},
					},
				},
			}
		}

		// Add error responses from handler's declared error definitions
		for _, errDef := range handler.ErrorDefs() {
			schemaName := "Err" + errorCodeToSchemaName(errDef.Code())
			operation.Responses[fmt.Sprintf("%d", errDef.Status())] = openapi.Response{
				Description: statusCodeToResponseName(errDef.Status()),
				Content: map[string]openapi.MediaType{
					ContentTypeJSON: {
						Schema: &openapi.Schema{Ref: "#/components/schemas/" + schemaName},
					},
				},
			}
		}

		// Add security requirements if handler requires authentication
		if handlerSpec.RequiresAuth {
			// Collect all required scopes (flattened from all groups)
			var allScopes []string
			for _, scopeGroup := range handlerSpec.ScopeGroups {
				allScopes = append(allScopes, scopeGroup...)
			}

			operation.Security = append(operation.Security, openapi.SecurityRequirement{
				"bearerAuth": allScopes, // Scopes for OAuth2/bearer tokens
			})
		}

		// Set operation on path item
		setOperationForMethod(&pathItem, handlerSpec.Method, operation)

		// Update paths
		spec.Paths[handlerSpec.Path] = pathItem
	}

	// Add collected schemas to components
	for name, schema := range schemas {
		spec.Components.Schemas[name] = schema
	}

	return spec
}
