package rocco

import (
	"net/http"
	"testing"

	"github.com/zoobz-io/openapi"
	"github.com/zoobz-io/sentinel"
)

func TestMetadataToSchema(t *testing.T) {
	meta := sentinel.Metadata{
		TypeName: "TestModel",
		Fields: []sentinel.FieldMetadata{
			{
				Name: "Name",
				Type: "string",
				Tags: map[string]string{
					"json": "name",
				},
			},
			{
				Name: "Count",
				Type: "int",
				Tags: map[string]string{
					"json": "count,omitempty",
				},
			},
		},
	}

	schema := metadataToSchema(meta)

	if schema.Type == nil || schema.Type.String() != "object" {
		t.Errorf("expected type 'object', got %v", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(schema.Properties))
	}
	if schema.Properties["name"].Type == nil || schema.Properties["name"].Type.String() != "string" {
		t.Errorf("expected name type 'string', got %v", schema.Properties["name"].Type)
	}
	if schema.Properties["count"].Type == nil || schema.Properties["count"].Type.String() != "integer" {
		t.Errorf("expected count type 'integer', got %v", schema.Properties["count"].Type)
	}
	// Name should be required, count should not (omitempty)
	if len(schema.Required) != 1 || schema.Required[0] != "name" {
		t.Errorf("expected required fields ['name'], got %v", schema.Required)
	}
}

func TestParseJSONTag(t *testing.T) {
	tests := []struct {
		field    sentinel.FieldMetadata
		wantName string
		wantReq  bool
	}{
		{
			sentinel.FieldMetadata{
				Name: "Field",
				Tags: map[string]string{"json": "field_name"},
			},
			"field_name",
			true,
		},
		{
			sentinel.FieldMetadata{
				Name: "Field",
				Tags: map[string]string{"json": "field_name,omitempty"},
			},
			"field_name",
			false,
		},
		{
			sentinel.FieldMetadata{
				Name: "Field",
				Tags: map[string]string{"json": "-"},
			},
			"-",
			true,
		},
		{
			sentinel.FieldMetadata{
				Name: "Field",
				Tags: map[string]string{},
			},
			"field",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			name, required := parseJSONTag(tt.field)
			if name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, name)
			}
			if required != tt.wantReq {
				t.Errorf("expected required %v, got %v", tt.wantReq, required)
			}
		})
	}
}

func TestGoTypeToSchema(t *testing.T) {
	tests := []struct {
		goType     string
		wantType   string
		wantFormat string
		wantItems  bool
	}{
		{"string", "string", "", false},
		{"int", "integer", "", false},
		{"int64", "integer", "", false},
		{"float64", "number", "", false},
		{"bool", "boolean", "", false},
		{"time.Time", "string", "date-time", false},
		{"[]string", "array", "", true},
		{"[]int", "array", "", true},
		{"map[string]string", "object", "", false},
		{"*string", "string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			schema := goTypeToSchema(tt.goType)
			schemaType := ""
			if schema.Type != nil {
				schemaType = schema.Type.String()
			}
			if schemaType != tt.wantType {
				t.Errorf("expected type %q, got %q", tt.wantType, schemaType)
			}
			if schema.Format != tt.wantFormat {
				t.Errorf("expected format %q, got %q", tt.wantFormat, schema.Format)
			}
			if tt.wantItems && schema.Items == nil {
				t.Error("expected items to be set")
			}
		})
	}
}

func TestGoTypeToSchema_MapValueType(t *testing.T) {
	t.Run("map[string]string", func(t *testing.T) {
		schema := goTypeToSchema("map[string]string")
		if schema.Type == nil || schema.Type.String() != "object" {
			t.Errorf("expected object type, got %v", schema.Type)
		}
		ap, ok := schema.AdditionalProperties.(*openapi.Schema)
		if !ok {
			t.Fatal("expected additionalProperties to be a schema")
		}
		if ap.Type == nil || ap.Type.String() != "string" {
			t.Errorf("expected string value type, got %v", ap.Type)
		}
	})

	t.Run("map[string]SomeStruct", func(t *testing.T) {
		schema := goTypeToSchema("map[string]SomeStruct")
		if schema.Type == nil || schema.Type.String() != "object" {
			t.Errorf("expected object type, got %v", schema.Type)
		}
		ap, ok := schema.AdditionalProperties.(*openapi.Schema)
		if !ok {
			t.Fatal("expected additionalProperties to be a schema")
		}
		if ap.Ref != "#/components/schemas/SomeStruct" {
			t.Errorf("expected $ref to SomeStruct, got %q", ap.Ref)
		}
	})

	t.Run("map[string][]int", func(t *testing.T) {
		schema := goTypeToSchema("map[string][]int")
		ap, ok := schema.AdditionalProperties.(*openapi.Schema)
		if !ok {
			t.Fatal("expected additionalProperties to be a schema")
		}
		if ap.Type == nil || ap.Type.String() != "array" {
			t.Errorf("expected array value type, got %v", ap.Type)
		}
		if ap.Items == nil || ap.Items.Type == nil || ap.Items.Type.String() != "integer" {
			t.Error("expected array items to be integer")
		}
	})
}

func TestGoTypeToSchema_ComplexType(t *testing.T) {
	schema := goTypeToSchema("github.com/user/pkg.CustomType")

	if schema.Ref != "#/components/schemas/CustomType" {
		t.Errorf("expected ref '#/components/schemas/CustomType', got %q", schema.Ref)
	}
}

func TestSchemaName(t *testing.T) {
	meta := sentinel.Metadata{
		TypeName: "UserModel",
	}

	name := schemaName(meta)
	if name != "UserModel" {
		t.Errorf("expected schema name 'UserModel', got %q", name)
	}
}

func TestStatusCodeToResponseName(t *testing.T) {
	tests := []struct {
		code int
		name string
	}{
		{400, "BadRequest"},
		{401, "Unauthorized"},
		{403, "Forbidden"},
		{404, "NotFound"},
		{409, "Conflict"},
		{422, "UnprocessableEntity"},
		{429, "TooManyRequests"},
		{500, "InternalServerError"},
		{999, "InternalServerError"}, // Unknown codes default to 500
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := statusCodeToResponseName(tt.code)
			if name != tt.name {
				t.Errorf("expected name %q, got %q", tt.name, name)
			}
		})
	}
}

func TestSetOperationForMethod(t *testing.T) {
	tests := []struct {
		method string
		check  func(*openapi.PathItem) bool
	}{
		{"GET", func(pi *openapi.PathItem) bool { return pi.Get != nil }},
		{"POST", func(pi *openapi.PathItem) bool { return pi.Post != nil }},
		{"PUT", func(pi *openapi.PathItem) bool { return pi.Put != nil }},
		{"DELETE", func(pi *openapi.PathItem) bool { return pi.Delete != nil }},
		{"PATCH", func(pi *openapi.PathItem) bool { return pi.Patch != nil }},
		{"OPTIONS", func(pi *openapi.PathItem) bool { return pi.Options != nil }},
		{"HEAD", func(pi *openapi.PathItem) bool { return pi.Head != nil }},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			pathItem := &openapi.PathItem{}
			operation := &openapi.Operation{OperationID: "test"}

			setOperationForMethod(pathItem, tt.method, operation)

			if !tt.check(pathItem) {
				t.Errorf("operation not set for method %s", tt.method)
			}
		})
	}
}

func TestGenerateOpenAPI(t *testing.T) {
	engine := newTestEngine()

	// Register test handlers
	handler1 := NewHandler[NoBody, testOutput](
		"get-test",
		"GET",
		"/test",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithSummary("Get test").WithTags("test")

	handler2 := NewHandler[testInput, testOutput](
		"create-test",
		"POST",
		"/test",
		func(req *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithSummary("Create test").WithTags("test").WithErrors(ErrBadRequest, ErrNotFound)

	engine.WithHandlers(handler1, handler2)

	// Set OpenAPI info
	engine.WithOpenAPIInfo(openapi.Info{
		Title:       "Test API",
		Version:     "1.0.0",
		Description: "Test API description",
	})

	// Generate OpenAPI spec
	spec := engine.GenerateOpenAPI(nil)

	// Check spec structure
	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI version '3.1.0', got %q", spec.OpenAPI)
	}
	if spec.Info.Title != "Test API" {
		t.Errorf("expected title 'Test API', got %q", spec.Info.Title)
	}

	// Check paths
	if len(spec.Paths) != 1 {
		t.Errorf("expected 1 path, got %d", len(spec.Paths))
	}
	pathItem, exists := spec.Paths["/test"]
	if !exists {
		t.Fatal("expected path '/test' to exist")
	}

	// Check GET operation
	if pathItem.Get == nil {
		t.Fatal("expected GET operation")
	}
	if pathItem.Get.OperationID != "get-test" {
		t.Errorf("expected operation ID 'get-test', got %q", pathItem.Get.OperationID)
	}
	if pathItem.Get.Summary != "Get test" {
		t.Errorf("expected summary 'Get test', got %q", pathItem.Get.Summary)
	}

	// Check POST operation
	if pathItem.Post == nil {
		t.Fatal("expected POST operation")
	}
	if pathItem.Post.OperationID != "create-test" {
		t.Errorf("expected operation ID 'create-test', got %q", pathItem.Post.OperationID)
	}
	if pathItem.Post.RequestBody == nil {
		t.Error("expected POST to have request body")
	}

	// Check error responses
	if len(pathItem.Post.Responses) < 3 {
		t.Errorf("expected at least 3 responses (200, 400, 404), got %d", len(pathItem.Post.Responses))
	}
	if _, exists := pathItem.Post.Responses["400"]; !exists {
		t.Error("expected 400 response")
	}
	if _, exists := pathItem.Post.Responses["404"]; !exists {
		t.Error("expected 404 response")
	}

	// Check components
	if spec.Components == nil {
		t.Fatal("expected components")
	}
	if len(spec.Components.Schemas) == 0 {
		t.Error("expected schemas in components")
	}

	// Check typed error schemas from declared errors
	if _, exists := spec.Components.Schemas["ErrBadRequest"]; !exists {
		t.Error("expected ErrBadRequest schema")
	}
	if _, exists := spec.Components.Schemas["ErrNotFound"]; !exists {
		t.Error("expected ErrNotFound schema")
	}
}

func TestGenerateOpenAPI_ErrorSchemaStructure(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"get-test",
		"GET",
		"/test",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithErrors(ErrBadRequest, ErrNotFound)

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	t.Run("code field has const value", func(t *testing.T) {
		schema := spec.Components.Schemas["ErrBadRequest"]
		if schema == nil {
			t.Fatal("expected ErrBadRequest schema")
		}
		codeSchema := schema.Properties["code"]
		if codeSchema == nil {
			t.Fatal("expected code property")
		}
		if codeSchema.Const != "BAD_REQUEST" {
			t.Errorf("expected code const 'BAD_REQUEST', got %v", codeSchema.Const)
		}

		schema = spec.Components.Schemas["ErrNotFound"]
		if schema == nil {
			t.Fatal("expected ErrNotFound schema")
		}
		codeSchema = schema.Properties["code"]
		if codeSchema == nil {
			t.Fatal("expected code property")
		}
		if codeSchema.Const != "NOT_FOUND" {
			t.Errorf("expected code const 'NOT_FOUND', got %v", codeSchema.Const)
		}
	})

	t.Run("required fields", func(t *testing.T) {
		schema := spec.Components.Schemas["ErrBadRequest"]
		if schema == nil {
			t.Fatal("expected ErrBadRequest schema")
		}
		hasCode, hasMessage := false, false
		for _, r := range schema.Required {
			if r == "code" {
				hasCode = true
			}
			if r == "message" {
				hasMessage = true
			}
		}
		if !hasCode || !hasMessage {
			t.Errorf("expected code and message in required, got %v", schema.Required)
		}
	})

	t.Run("details inlined as object", func(t *testing.T) {
		schema := spec.Components.Schemas["ErrNotFound"]
		if schema == nil {
			t.Fatal("expected ErrNotFound schema")
		}
		details := schema.Properties["details"]
		if details == nil {
			t.Fatal("expected details property")
		}
		if details.Ref != "" {
			t.Errorf("expected details to be inlined, got $ref %q", details.Ref)
		}
		if details.Type == nil || details.Type.String() != "object" {
			t.Error("expected details to be an object type")
		}
		if _, exists := details.Properties["resource"]; !exists {
			t.Error("expected details to have 'resource' property from NotFoundDetails")
		}
	})

	t.Run("no separate details models registered", func(t *testing.T) {
		for name := range spec.Components.Schemas {
			if name == "BadRequestDetails" || name == "NotFoundDetails" {
				t.Errorf("details type %q should not be registered as a separate schema", name)
			}
		}
	})

	t.Run("no base ErrorResponse schema", func(t *testing.T) {
		if _, exists := spec.Components.Schemas["ErrorResponse"]; exists {
			t.Error("ErrorResponse base schema should not be registered")
		}
	})
}

func TestGenerateOpenAPI_CustomErrorSchema(t *testing.T) {
	type TeapotDetails struct {
		TeaType string `json:"tea_type" description:"The type of tea requested"`
		Temp    int    `json:"temp" description:"Temperature in celsius"`
	}

	errTeapot := NewError[TeapotDetails]("TEAPOT", 418, "I'm a teapot")

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"brew",
		"POST",
		"/brew",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithErrors(errTeapot)

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	schema := spec.Components.Schemas["ErrTeapot"]
	if schema == nil {
		t.Fatal("expected ErrTeapot schema")
	}
	if schema.Properties["code"].Const != "TEAPOT" {
		t.Errorf("expected code const 'TEAPOT', got %v", schema.Properties["code"].Const)
	}
	details := schema.Properties["details"]
	if details == nil {
		t.Fatal("expected details property")
	}
	if _, exists := details.Properties["tea_type"]; !exists {
		t.Error("expected details to have 'tea_type' property")
	}
	if _, exists := details.Properties["temp"]; !exists {
		t.Error("expected details to have 'temp' property")
	}
}

func TestGenerateOpenAPI_ErrorDetailsNestedTypes(t *testing.T) {
	type NestedField struct {
		Code    string `json:"code" description:"Field error code"`
		Message string `json:"message" description:"Field error message"`
	}
	type DetailsWithNested struct {
		Fields []NestedField `json:"fields" description:"Per-field errors"`
	}

	errNested := NewError[DetailsWithNested]("NESTED_ERROR", 422, "has nested details")

	engine := newTestEngine()
	handler := NewHandler[NoBody, testOutput](
		"nested",
		"POST",
		"/nested",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithErrors(errNested)

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	// The nested type should be registered as a component schema
	nestedSchema := spec.Components.Schemas["NestedField"]
	if nestedSchema == nil {
		t.Fatal("expected NestedField to be registered as a component schema")
	}
	if _, exists := nestedSchema.Properties["code"]; !exists {
		t.Error("expected NestedField to have 'code' property")
	}
	if _, exists := nestedSchema.Properties["message"]; !exists {
		t.Error("expected NestedField to have 'message' property")
	}

	// The details type itself should NOT be registered (it's inlined)
	if _, exists := spec.Components.Schemas["DetailsWithNested"]; exists {
		t.Error("details type should be inlined, not registered as a separate schema")
	}
}

func TestGenerateOpenAPI_ErrorDeduplication(t *testing.T) {
	engine := newTestEngine()

	// Declare the same error twice via separate WithErrors calls
	handler := NewHandler[NoBody, testOutput](
		"test",
		"GET",
		"/test",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithErrors(ErrBadRequest).WithErrors(ErrBadRequest, ErrNotFound)

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	// Should have exactly 2 error responses (400, 404) plus 200
	pathItem := spec.Paths["/test"]
	if pathItem.Get == nil {
		t.Fatal("expected GET operation")
	}
	if len(pathItem.Get.Responses) != 3 {
		t.Errorf("expected 3 responses (200, 400, 404), got %d", len(pathItem.Get.Responses))
	}

	// Declared error definitions should be deduplicated by code
	if len(handler.ErrorDefs()) != 2 {
		t.Errorf("expected 2 error definitions, got %d", len(handler.ErrorDefs()))
	}
}

func TestGenerateOpenAPI_PathParams(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"get-user",
		"GET",
		"/users/{id}",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithPathParams("id")

	engine.WithHandlers(handler)

	engine.WithOpenAPIInfo(openapi.Info{Title: "Test", Version: "1.0.0"})
	spec := engine.GenerateOpenAPI(nil)

	pathItem := spec.Paths["/users/{id}"]
	if pathItem.Get == nil {
		t.Fatal("expected GET operation")
	}
	if len(pathItem.Get.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(pathItem.Get.Parameters))
	}

	param := pathItem.Get.Parameters[0]
	if param.Name != "id" {
		t.Errorf("expected parameter name 'id', got %q", param.Name)
	}
	if param.In != "path" {
		t.Errorf("expected parameter in 'path', got %q", param.In)
	}
	if !param.Required {
		t.Error("expected path parameter to be required")
	}
}

func TestGenerateOpenAPI_QueryParams(t *testing.T) {
	engine := newTestEngine()

	handler := NewHandler[NoBody, testOutput](
		"list-users",
		"GET",
		"/users",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithQueryParams("page", "limit")

	engine.WithHandlers(handler)

	engine.WithOpenAPIInfo(openapi.Info{Title: "Test", Version: "1.0.0"})
	spec := engine.GenerateOpenAPI(nil)

	pathItem := spec.Paths["/users"]
	if pathItem.Get == nil {
		t.Fatal("expected GET operation")
	}
	if len(pathItem.Get.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(pathItem.Get.Parameters))
	}

	// Check both query params exist
	paramNames := make(map[string]bool)
	for _, param := range pathItem.Get.Parameters {
		paramNames[param.Name] = true
		if param.In != "query" {
			t.Errorf("expected parameter in 'query', got %q", param.In)
		}
		if param.Required {
			t.Error("expected query parameter to not be required")
		}
	}
	if !paramNames["page"] || !paramNames["limit"] {
		t.Error("expected 'page' and 'limit' parameters")
	}
}

func TestApplyOpenAPITags_Description(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Name",
		Type: "string",
		Tags: map[string]string{
			"description": "User's full name",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Description != "User's full name" {
		t.Errorf("expected description 'User's full name', got %q", schema.Description)
	}
}

func TestApplyOpenAPITags_Format(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Email",
		Type: "string",
		Tags: map[string]string{
			"validate": "email",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Format != "email" {
		t.Errorf("expected format 'email', got %q", schema.Format)
	}
}

func TestApplyOpenAPITags_Example(t *testing.T) {
	tests := []struct {
		name         string
		schemaType   string
		exampleValue string
		want         any
	}{
		{"string", "string", "hello", "hello"},
		{"integer", "integer", "42", 42},
		{"number", "number", "3.14", 3.14},
		{"boolean", "boolean", "true", true},
		{"array", "array", "a,b,c", []any{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := sentinel.FieldMetadata{
				Name: "Field",
				Type: tt.schemaType,
				Tags: map[string]string{
					"example": tt.exampleValue,
				},
			}

			schema := &openapi.Schema{Type: openapi.NewSchemaType(tt.schemaType)}
			applyOpenAPITags(schema, field)

			if schema.Example == nil {
				t.Fatal("expected example to be set")
			}

			// Compare based on type
			switch want := tt.want.(type) {
			case string:
				if got, ok := schema.Example.(string); !ok || got != want {
					t.Errorf("expected example %v, got %v", want, schema.Example)
				}
			case int:
				if got, ok := schema.Example.(int); !ok || got != want {
					t.Errorf("expected example %v, got %v", want, schema.Example)
				}
			case float64:
				if got, ok := schema.Example.(float64); !ok || got != want {
					t.Errorf("expected example %v, got %v", want, schema.Example)
				}
			case bool:
				if got, ok := schema.Example.(bool); !ok || got != want {
					t.Errorf("expected example %v, got %v", want, schema.Example)
				}
			}
		})
	}
}

func TestApplyOpenAPITags_Pattern(t *testing.T) {
	// Note: pattern validation is not supported via validate tags
	// This test is kept for backward compatibility with custom tags if needed
	t.Skip("Pattern validation is not extracted from validate tags")
}

func TestApplyOpenAPITags_Enum(t *testing.T) {
	tests := []struct {
		name        string
		schemaType  string
		validateTag string
		wantLen     int
	}{
		{"string", "string", "oneof=red green blue", 3},
		{"integer", "integer", "oneof=1 2 3", 3},
		{"number", "number", "oneof=1.5 2.5 3.5", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := sentinel.FieldMetadata{
				Name: "Field",
				Type: tt.schemaType,
				Tags: map[string]string{
					"validate": tt.validateTag,
				},
			}

			schema := &openapi.Schema{Type: openapi.NewSchemaType(tt.schemaType)}
			applyOpenAPITags(schema, field)

			if len(schema.Enum) != tt.wantLen {
				t.Errorf("expected %d enum values, got %d", tt.wantLen, len(schema.Enum))
			}
		})
	}
}

func TestApplyOpenAPITags_NumericValidations(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Age",
		Type: "int",
		Tags: map[string]string{
			"validate": "min=0,max=120",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("integer")}
	applyOpenAPITags(schema, field)

	if schema.Minimum == nil || *schema.Minimum != 0 {
		t.Errorf("expected minimum 0, got %v", schema.Minimum)
	}
	if schema.Maximum == nil || *schema.Maximum != 120 {
		t.Errorf("expected maximum 120, got %v", schema.Maximum)
	}
	// Note: multipleOf is not supported via validate tags
}

func TestApplyOpenAPITags_StringValidations(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Username",
		Type: "string",
		Tags: map[string]string{
			"validate": "min=3,max=20",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.MinLength == nil || *schema.MinLength != 3 {
		t.Errorf("expected minLength 3, got %v", schema.MinLength)
	}
	if schema.MaxLength == nil || *schema.MaxLength != 20 {
		t.Errorf("expected maxLength 20, got %v", schema.MaxLength)
	}
}

func TestApplyOpenAPITags_ArrayValidations(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Tags",
		Type: "[]string",
		Tags: map[string]string{
			"validate": "len=5,unique",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("array")}
	applyOpenAPITags(schema, field)

	if schema.MinItems == nil || *schema.MinItems != 5 {
		t.Errorf("expected minItems 5, got %v", schema.MinItems)
	}
	if schema.MaxItems == nil || *schema.MaxItems != 5 {
		t.Errorf("expected maxItems 5, got %v", schema.MaxItems)
	}
	if schema.UniqueItems == nil || *schema.UniqueItems != true {
		t.Errorf("expected uniqueItems true, got %v", schema.UniqueItems)
	}
}

func TestApplyOpenAPITags_BooleanFlags(t *testing.T) {
	// Note: readOnly, nullable, deprecated are not supported via validate tags
	// These would need custom sentinel tags if needed
	t.Skip("Boolean flags are not extracted from validate tags")
}

func TestApplyOpenAPITags_MultipleTagsCombined(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Email",
		Type: "string",
		Tags: map[string]string{
			"description": "User email address",
			"validate":    "email,min=5,max=100",
			"example":     "user@example.com",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Description != "User email address" {
		t.Errorf("expected description, got %q", schema.Description)
	}
	if schema.Format != "email" {
		t.Errorf("expected format email, got %q", schema.Format)
	}
	if schema.Example != "user@example.com" {
		t.Errorf("expected example, got %v", schema.Example)
	}
	if schema.MinLength == nil || *schema.MinLength != 5 {
		t.Errorf("expected minLength 5, got %v", schema.MinLength)
	}
	if schema.MaxLength == nil || *schema.MaxLength != 100 {
		t.Errorf("expected maxLength 100, got %v", schema.MaxLength)
	}
}

func TestParseFloat64(t *testing.T) {
	tests := []struct {
		input string
		want  *float64
	}{
		{"", nil},
		{"invalid", nil},
		{"3.14", float64Ptr(3.14)},
		{"0", float64Ptr(0)},
		{"-10.5", float64Ptr(-10.5)},
	}

	for _, tt := range tests {
		got := parseFloat64(tt.input)
		if (got == nil) != (tt.want == nil) {
			t.Errorf("parseFloat64(%q) = %v, want %v", tt.input, got, tt.want)
		} else if got != nil && *got != *tt.want {
			t.Errorf("parseFloat64(%q) = %v, want %v", tt.input, *got, *tt.want)
		}
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  *int
	}{
		{"", nil},
		{"invalid", nil},
		{"42", intPtr(42)},
		{"0", intPtr(0)},
		{"-10", intPtr(-10)},
	}

	for _, tt := range tests {
		got := parseInt(tt.input)
		if (got == nil) != (tt.want == nil) {
			t.Errorf("parseInt(%q) = %v, want %v", tt.input, got, tt.want)
		} else if got != nil && *got != *tt.want {
			t.Errorf("parseInt(%q) = %v, want %v", tt.input, *got, *tt.want)
		}
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input string
		want  *bool
	}{
		{"", nil},
		{"invalid", nil},
		{"true", boolPtr(true)},
		{"false", boolPtr(false)},
		{"1", boolPtr(true)},
		{"0", boolPtr(false)},
	}

	for _, tt := range tests {
		got := parseBool(tt.input)
		if (got == nil) != (tt.want == nil) {
			t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.want)
		} else if got != nil && *got != *tt.want {
			t.Errorf("parseBool(%q) = %v, want %v", tt.input, *got, *tt.want)
		}
	}
}

func TestParseEnum(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		schemaType string
		wantLen    int
	}{
		{"empty", "", "string", 0},
		{"string", "red,green,blue", "string", 3},
		{"integer", "1,2,3", "integer", 3},
		{"number", "1.5,2.5", "number", 2},
		{"boolean", "true,false", "boolean", 2},
		{"with spaces", "a, b, c", "string", 3},
		{"with empty", "a,,b", "string", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnum(tt.value, tt.schemaType)
			if (got == nil && tt.wantLen > 0) || (got != nil && len(got) != tt.wantLen) {
				t.Errorf("parseEnum(%q, %q) length = %v, want %d", tt.value, tt.schemaType, len(got), tt.wantLen)
			}
		})
	}
}

func TestParseExample(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		schemaType string
		wantType   string
	}{
		{"string", "hello", "string", "string"},
		{"integer", "42", "integer", "int"},
		{"number", "3.14", "number", "float64"},
		{"boolean true", "true", "boolean", "bool"},
		{"boolean false", "false", "boolean", "bool"},
		{"array", "a,b,c", "array", "slice"},
		{"empty", "", "string", "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExample(tt.value, tt.schemaType)

			if tt.wantType == "nil" {
				if got != nil {
					t.Errorf("parseExample(%q, %q) = %v, want nil", tt.value, tt.schemaType, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("parseExample(%q, %q) = nil, want %s", tt.value, tt.schemaType, tt.wantType)
			}

			switch tt.wantType {
			case "string":
				if _, ok := got.(string); !ok {
					t.Errorf("parseExample(%q, %q) type = %T, want string", tt.value, tt.schemaType, got)
				}
			case "int":
				if _, ok := got.(int); !ok {
					t.Errorf("parseExample(%q, %q) type = %T, want int", tt.value, tt.schemaType, got)
				}
			case "float64":
				if _, ok := got.(float64); !ok {
					t.Errorf("parseExample(%q, %q) type = %T, want float64", tt.value, tt.schemaType, got)
				}
			case "bool":
				if _, ok := got.(bool); !ok {
					t.Errorf("parseExample(%q, %q) type = %T, want bool", tt.value, tt.schemaType, got)
				}
			case "slice":
				if _, ok := got.([]any); !ok {
					t.Errorf("parseExample(%q, %q) type = %T, want []any", tt.value, tt.schemaType, got)
				}
			}
		})
	}
}

// Helper functions to create pointers
func intPtr(i int) *int             { return &i }
func float64Ptr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool          { return &b }

func TestParseJSONTag_EmptyName(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "MyField",
		Tags: map[string]string{"json": ",omitempty"},
	}

	name, required := parseJSONTag(field)

	if name != "myfield" {
		t.Errorf("expected name 'myfield', got %q", name)
	}
	if required {
		t.Error("expected required=false with omitempty")
	}
}

func TestMetadataToSchema_SkipJSONDashFields(t *testing.T) {
	meta := sentinel.Metadata{
		TypeName: "TestModel",
		Fields: []sentinel.FieldMetadata{
			{
				Name: "IncludedField",
				Type: "string",
				Tags: map[string]string{
					"json": "included",
				},
			},
			{
				Name: "SkippedField",
				Type: "string",
				Tags: map[string]string{
					"json": "-",
				},
			},
		},
	}

	schema := metadataToSchema(meta)

	if len(schema.Properties) != 1 {
		t.Errorf("expected 1 property (skipping json:\"-\"), got %d", len(schema.Properties))
	}
	if _, exists := schema.Properties["included"]; !exists {
		t.Error("expected 'included' property to exist")
	}
	if _, exists := schema.Properties["-"]; exists {
		t.Error("did not expect '-' property to exist")
	}
}

func TestParseValidateTag_GteLte(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Score",
		Type: "int",
		Tags: map[string]string{
			"validate": "gte=0,lte=100",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("integer")}
	applyOpenAPITags(schema, field)

	if schema.Minimum == nil || *schema.Minimum != 0 {
		t.Errorf("expected minimum 0 from gte, got %v", schema.Minimum)
	}
	if schema.Maximum == nil || *schema.Maximum != 100 {
		t.Errorf("expected maximum 100 from lte, got %v", schema.Maximum)
	}
}

func TestParseValidateTag_GtLt(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Value",
		Type: "float64",
		Tags: map[string]string{
			"validate": "gt=0,lt=1",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("number")}
	applyOpenAPITags(schema, field)

	// OpenAPI 3.1.0: exclusiveMinimum/Maximum are the actual bound values, not booleans
	if schema.ExclusiveMinimum == nil || *schema.ExclusiveMinimum != 0 {
		t.Errorf("expected exclusiveMinimum 0 from gt, got %v", schema.ExclusiveMinimum)
	}
	if schema.ExclusiveMaximum == nil || *schema.ExclusiveMaximum != 1 {
		t.Errorf("expected exclusiveMaximum 1 from lt, got %v", schema.ExclusiveMaximum)
	}
}

func TestParseValidateTag_StringLen(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Code",
		Type: "string",
		Tags: map[string]string{
			"validate": "len=5",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.MinLength == nil || *schema.MinLength != 5 {
		t.Errorf("expected minLength 5 from len, got %v", schema.MinLength)
	}
	if schema.MaxLength == nil || *schema.MaxLength != 5 {
		t.Errorf("expected maxLength 5 from len, got %v", schema.MaxLength)
	}
}

func TestParseValidateTag_URLFormat(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Website",
		Type: "string",
		Tags: map[string]string{
			"validate": "url",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Format != "uri" {
		t.Errorf("expected format 'uri' from url validation, got %q", schema.Format)
	}
}

func TestParseValidateTag_UUIDFormats(t *testing.T) {
	tests := []string{"uuid", "uuid4", "uuid5"}

	for _, validateTag := range tests {
		t.Run(validateTag, func(t *testing.T) {
			field := sentinel.FieldMetadata{
				Name: "ID",
				Type: "string",
				Tags: map[string]string{
					"validate": validateTag,
				},
			}

			schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
			applyOpenAPITags(schema, field)

			if schema.Format != "uuid" {
				t.Errorf("expected format 'uuid' from %s validation, got %q", validateTag, schema.Format)
			}
		})
	}
}

func TestParseValidateTag_IPFormats(t *testing.T) {
	tests := []struct {
		tag    string
		format string
	}{
		{"ipv4", "ipv4"},
		{"ipv6", "ipv6"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			field := sentinel.FieldMetadata{
				Name: "Address",
				Type: "string",
				Tags: map[string]string{
					"validate": tt.tag,
				},
			}

			schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
			applyOpenAPITags(schema, field)

			if schema.Format != tt.format {
				t.Errorf("expected format %q from %s validation, got %q", tt.format, tt.tag, schema.Format)
			}
		})
	}
}

func TestParseValidateTag_DateTime(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "CreatedAt",
		Type: "string",
		Tags: map[string]string{
			"validate": "datetime",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Format != "date-time" {
		t.Errorf("expected format 'date-time' from datetime validation, got %q", schema.Format)
	}
}

func TestParseEnum_InvalidNumbers(t *testing.T) {
	// Test that invalid integer/float values are skipped
	tests := []struct {
		name       string
		value      string
		schemaType string
		wantLen    int
	}{
		{"invalid integers", "1,invalid,3", "integer", 2},
		{"invalid floats", "1.5,invalid,3.5", "number", 2},
		{"invalid booleans", "true,invalid,false", "boolean", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnum(tt.value, tt.schemaType)
			if len(got) != tt.wantLen {
				t.Errorf("parseEnum(%q, %q) length = %d, want %d", tt.value, tt.schemaType, len(got), tt.wantLen)
			}
		})
	}
}

// TestGenerateOpenAPI_Filtering tests that GenerateOpenAPI filters handlers based on identity permissions
func TestGenerateOpenAPI_Filtering(t *testing.T) {
	engine := NewEngine()

	publicHandler := NewHandler[NoBody, testOutput](
		"public",
		"GET",
		"/public",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "public"}, nil
		},
	)

	readHandler := NewHandler[NoBody, testOutput](
		"read",
		"GET",
		"/read",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "read"}, nil
		},
	).WithScopes("read")

	adminHandler := NewHandler[NoBody, testOutput](
		"admin",
		"DELETE",
		"/admin",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "admin"}, nil
		},
	).WithRoles("admin")

	engine.WithHandlers(publicHandler, readHandler, adminHandler)

	// Test: No identity - all handlers visible
	spec := engine.GenerateOpenAPI(nil)
	if len(spec.Paths) != 3 {
		t.Errorf("no identity: expected 3 paths, got %d", len(spec.Paths))
	}

	// Test: Identity with read scope - public + read visible
	readIdentity := &mockIdentity{scopes: []string{"read"}}
	spec = engine.GenerateOpenAPI(readIdentity)
	if len(spec.Paths) != 2 {
		t.Errorf("read identity: expected 2 paths, got %d", len(spec.Paths))
	}
	if _, exists := spec.Paths["/public"]; !exists {
		t.Error("read identity: expected /public to exist")
	}
	if _, exists := spec.Paths["/read"]; !exists {
		t.Error("read identity: expected /read to exist")
	}
	if _, exists := spec.Paths["/admin"]; exists {
		t.Error("read identity: expected /admin to NOT exist")
	}

	// Test: Identity with no permissions - only public visible
	noPermIdentity := &mockIdentity{}
	spec = engine.GenerateOpenAPI(noPermIdentity)
	if len(spec.Paths) != 1 {
		t.Errorf("no permissions: expected 1 path, got %d", len(spec.Paths))
	}
	if _, exists := spec.Paths["/public"]; !exists {
		t.Error("no permissions: expected /public to exist")
	}

	// Test: Identity with admin role - public + admin visible
	adminIdentity := &mockIdentity{roles: []string{"admin"}}
	spec = engine.GenerateOpenAPI(adminIdentity)
	if len(spec.Paths) != 2 {
		t.Errorf("admin identity: expected 2 paths, got %d", len(spec.Paths))
	}
	if _, exists := spec.Paths["/public"]; !exists {
		t.Error("admin identity: expected /public to exist")
	}
	if _, exists := spec.Paths["/admin"]; !exists {
		t.Error("admin identity: expected /admin to exist")
	}
	if _, exists := spec.Paths["/read"]; exists {
		t.Error("admin identity: expected /read to NOT exist (needs read scope)")
	}
}

// TestGenerateOpenAPI_WithTags tests engine-level tag descriptions
func TestGenerateOpenAPI_WithTags(t *testing.T) {
	engine := NewEngine()
	engine.WithTag("users", "User management endpoints")
	engine.WithTag("posts", "Blog post endpoints")

	handler := NewHandler[NoBody, testOutput](
		"get-user",
		"GET",
		"/users",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "user"}, nil
		},
	).WithTags("users")

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	if len(spec.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(spec.Tags))
	}

	foundUsers := false
	for _, tag := range spec.Tags {
		if tag.Name == "users" && tag.Description == "User management endpoints" {
			foundUsers = true
		}
	}
	if !foundUsers {
		t.Error("expected users tag with description")
	}
}

// TestGenerateOpenAPI_WithTagGroups tests that tag groups appear in generated spec
func TestGenerateOpenAPI_WithTagGroups(t *testing.T) {
	engine := NewEngine()
	engine.WithTag("auth", "Authentication endpoints")
	engine.WithTag("users", "User management endpoints")
	engine.WithTagGroup("Account", "auth", "users")

	handler := NewHandler[NoBody, testOutput](
		"get-user",
		"GET",
		"/users",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "user"}, nil
		},
	).WithTags("users")

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	if len(spec.TagGroups) != 1 {
		t.Fatalf("expected 1 tag group, got %d", len(spec.TagGroups))
	}
	if spec.TagGroups[0].Name != "Account" {
		t.Errorf("expected tag group name 'Account', got %q", spec.TagGroups[0].Name)
	}
	if len(spec.TagGroups[0].Tags) != 2 {
		t.Errorf("expected 2 tags in group, got %d", len(spec.TagGroups[0].Tags))
	}
	if spec.TagGroups[0].Tags[0] != "auth" || spec.TagGroups[0].Tags[1] != "users" {
		t.Errorf("expected tags [auth, users], got %v", spec.TagGroups[0].Tags)
	}
}

type mockIdentity struct {
	scopes []string
	roles  []string
}

func (m *mockIdentity) ID() string            { return "test-id" }
func (m *mockIdentity) TenantID() string      { return "test-tenant" }
func (m *mockIdentity) Email() string         { return "" }
func (m *mockIdentity) Scopes() []string      { return m.scopes }
func (m *mockIdentity) Roles() []string       { return m.roles }
func (m *mockIdentity) Stats() map[string]int { return nil }
func (m *mockIdentity) HasScope(scope string) bool {
	for _, s := range m.scopes {
		if s == scope {
			return true
		}
	}
	return false
}
func (m *mockIdentity) HasRole(role string) bool {
	for _, r := range m.roles {
		if r == role {
			return true
		}
	}
	return false
}

func TestGenerateOpenAPI_EmptyContentTypeFallback(t *testing.T) {
	engine := newTestEngine()

	// Create a handler and clear its media types to exercise the fallback
	handler := NewHandler[testInput, testOutput](
		"empty-ct",
		"POST",
		"/empty-ct",
		func(req *Request[testInput]) (testOutput, error) {
			return testOutput{}, nil
		},
	)
	handler.spec.Request.MediaTypes = nil // Force empty to trigger fallback
	handler.spec.Response.MediaTypes = nil

	engine.WithHandlers(handler)
	engine.WithOpenAPIInfo(openapi.Info{Title: "Test", Version: "1.0.0"})

	spec := engine.GenerateOpenAPI(nil)

	pathItem, exists := spec.Paths["/empty-ct"]
	if !exists {
		t.Fatal("expected path '/empty-ct' to exist")
	}
	if pathItem.Post == nil {
		t.Fatal("expected POST operation")
	}

	// Check request body uses fallback content type
	if pathItem.Post.RequestBody == nil {
		t.Fatal("expected request body")
	}
	if _, exists := pathItem.Post.RequestBody.Content[ContentTypeJSON]; !exists {
		t.Error("expected request body content type to fall back to application/json")
	}

	// Check response uses fallback content type
	resp, exists := pathItem.Post.Responses["200"]
	if !exists {
		t.Fatal("expected 200 response")
	}
	if _, exists := resp.Content[ContentTypeJSON]; !exists {
		t.Error("expected response content type to fall back to application/json")
	}
}

func TestParseValidateTag_EmptyTag(t *testing.T) {
	constraints := parseValidateTag("", "string")
	if constraints != nil {
		t.Errorf("expected nil for empty tag, got %v", constraints)
	}
}

func TestParseValidateTag_PointerTypes(t *testing.T) {
	// Test that pointer types are handled correctly
	constraints := parseValidateTag("min=5,max=50", "*string")

	if minLen := constraints["minLength"].(*int); *minLen != 5 {
		t.Errorf("minLength = %v, want 5", *minLen)
	}

	if maxLen := constraints["maxLength"].(*int); *maxLen != 50 {
		t.Errorf("maxLength = %v, want 50", *maxLen)
	}
}

func TestParseValidateTag_NumericConstraints(t *testing.T) {
	tests := []struct {
		name        string
		validateTag string
		goType      string
		wantMin     *float64
		wantMax     *float64
		wantExclMin *float64
		wantExclMax *float64
	}{
		{
			name:        "min constraint on int",
			validateTag: "min=0",
			goType:      "int",
			wantMin:     float64Ptr(0),
		},
		{
			name:        "max constraint on int",
			validateTag: "max=100",
			goType:      "int",
			wantMax:     float64Ptr(100),
		},
		{
			name:        "min and max on float64",
			validateTag: "min=0.5,max=99.5",
			goType:      "float64",
			wantMin:     float64Ptr(0.5),
			wantMax:     float64Ptr(99.5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := parseValidateTag(tt.validateTag, tt.goType)

			if tt.wantMin != nil {
				minVal := constraints["minimum"].(*float64)
				if *minVal != *tt.wantMin {
					t.Errorf("minimum = %v, want %v", *minVal, *tt.wantMin)
				}
			} else if _, exists := constraints["minimum"]; exists {
				t.Error("unexpected minimum constraint")
			}

			if tt.wantMax != nil {
				maxVal := constraints["maximum"].(*float64)
				if *maxVal != *tt.wantMax {
					t.Errorf("maximum = %v, want %v", *maxVal, *tt.wantMax)
				}
			} else if _, exists := constraints["maximum"]; exists {
				t.Error("unexpected maximum constraint")
			}
		})
	}
}

func TestParseValidateTag_StringConstraints(t *testing.T) {
	tests := []struct {
		name        string
		validateTag string
		goType      string
		wantMinLen  *int
		wantMaxLen  *int
		wantFormat  string
	}{
		{
			name:        "min length",
			validateTag: "min=3",
			goType:      "string",
			wantMinLen:  intPtr(3),
		},
		{
			name:        "max length",
			validateTag: "max=50",
			goType:      "string",
			wantMaxLen:  intPtr(50),
		},
		{
			name:        "min and max length",
			validateTag: "min=5,max=100",
			goType:      "string",
			wantMinLen:  intPtr(5),
			wantMaxLen:  intPtr(100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := parseValidateTag(tt.validateTag, tt.goType)

			if tt.wantMinLen != nil {
				minLen := constraints["minLength"].(*int)
				if *minLen != *tt.wantMinLen {
					t.Errorf("minLength = %v, want %v", *minLen, *tt.wantMinLen)
				}
			}

			if tt.wantMaxLen != nil {
				maxLen := constraints["maxLength"].(*int)
				if *maxLen != *tt.wantMaxLen {
					t.Errorf("maxLength = %v, want %v", *maxLen, *tt.wantMaxLen)
				}
			}

			if tt.wantFormat != "" {
				format := constraints["format"].(string)
				if format != tt.wantFormat {
					t.Errorf("format = %q, want %q", format, tt.wantFormat)
				}
			}
		})
	}
}

func TestParseValidateTag_ArrayConstraints(t *testing.T) {
	tests := []struct {
		name         string
		validateTag  string
		goType       string
		wantMinItems *int
		wantMaxItems *int
		wantUnique   *bool
	}{
		{
			name:         "exact length",
			validateTag:  "len=5",
			goType:       "[]string",
			wantMinItems: intPtr(5),
			wantMaxItems: intPtr(5),
		},
		{
			name:        "unique items",
			validateTag: "unique",
			goType:      "[]string",
			wantUnique:  boolPtr(true),
		},
		{
			name:         "combined constraints",
			validateTag:  "len=5,unique",
			goType:       "[]string",
			wantMinItems: intPtr(5),
			wantMaxItems: intPtr(5),
			wantUnique:   boolPtr(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := parseValidateTag(tt.validateTag, tt.goType)

			if tt.wantMinItems != nil {
				val, exists := constraints["minItems"]
				if !exists {
					t.Error("expected minItems constraint")
				} else {
					minItems := val.(*int)
					if *minItems != *tt.wantMinItems {
						t.Errorf("minItems = %v, want %v", *minItems, *tt.wantMinItems)
					}
				}
			}

			if tt.wantMaxItems != nil {
				val, exists := constraints["maxItems"]
				if !exists {
					t.Error("expected maxItems constraint")
				} else {
					maxItems := val.(*int)
					if *maxItems != *tt.wantMaxItems {
						t.Errorf("maxItems = %v, want %v", *maxItems, *tt.wantMaxItems)
					}
				}
			}

			if tt.wantUnique != nil {
				val, exists := constraints["uniqueItems"]
				if !exists {
					t.Error("expected uniqueItems constraint")
				} else {
					unique := val.(*bool)
					if *unique != *tt.wantUnique {
						t.Errorf("uniqueItems = %v, want %v", *unique, *tt.wantUnique)
					}
				}
			}
		})
	}
}

func TestParseValidateTag_Enum(t *testing.T) {
	tests := []struct {
		name        string
		validateTag string
		goType      string
		wantEnum    []any
	}{
		{
			name:        "string enum",
			validateTag: "oneof=red green blue",
			goType:      "string",
			wantEnum:    []any{"red", "green", "blue"},
		},
		{
			name:        "integer enum",
			validateTag: "oneof=1 2 3",
			goType:      "int",
			wantEnum:    []any{1, 2, 3},
		},
		{
			name:        "float enum",
			validateTag: "oneof=1.5 2.5 3.5",
			goType:      "float64",
			wantEnum:    []any{1.5, 2.5, 3.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := parseValidateTag(tt.validateTag, tt.goType)

			enum, exists := constraints["enum"]
			if !exists {
				t.Fatal("expected enum constraint")
			}

			enumVals := enum.([]any)
			if len(enumVals) != len(tt.wantEnum) {
				t.Fatalf("enum length = %d, want %d", len(enumVals), len(tt.wantEnum))
			}

			for i, want := range tt.wantEnum {
				got := enumVals[i]
				if got != want {
					t.Errorf("enum[%d] = %v, want %v", i, got, want)
				}
			}
		})
	}
}

func TestParseValidateTag_Combined(t *testing.T) {
	constraints := parseValidateTag("min=1,max=100,required", "int")

	if minVal := constraints["minimum"].(*float64); *minVal != 1 {
		t.Errorf("minimum = %v, want 1", *minVal)
	}

	if maxVal := constraints["maximum"].(*float64); *maxVal != 100 {
		t.Errorf("maximum = %v, want 100", *maxVal)
	}
}

func TestBuildDiscriminatedUnionSchema(t *testing.T) {
	discriminators := map[string]string{
		"event": "type",
	}

	schema := buildDiscriminatedUnionSchema("event", "IngestCompletedEvent,IngestFailedEvent", discriminators)

	t.Run("oneOf refs", func(t *testing.T) {
		if len(schema.OneOf) != 2 {
			t.Fatalf("expected 2 oneOf entries, got %d", len(schema.OneOf))
		}
		if schema.OneOf[0].Ref != "#/components/schemas/IngestCompletedEvent" {
			t.Errorf("expected ref to IngestCompletedEvent, got %q", schema.OneOf[0].Ref)
		}
		if schema.OneOf[1].Ref != "#/components/schemas/IngestFailedEvent" {
			t.Errorf("expected ref to IngestFailedEvent, got %q", schema.OneOf[1].Ref)
		}
	})

	t.Run("discriminator", func(t *testing.T) {
		if schema.Discriminator == nil {
			t.Fatal("expected discriminator to be set")
		}
		if schema.Discriminator.PropertyName != "type" {
			t.Errorf("expected propertyName 'type', got %q", schema.Discriminator.PropertyName)
		}
		if len(schema.Discriminator.Mapping) != 2 {
			t.Fatalf("expected 2 mapping entries, got %d", len(schema.Discriminator.Mapping))
		}
		if schema.Discriminator.Mapping["IngestCompletedEvent"] != "#/components/schemas/IngestCompletedEvent" {
			t.Errorf("unexpected mapping for IngestCompletedEvent: %q", schema.Discriminator.Mapping["IngestCompletedEvent"])
		}
	})
}

func TestBuildDiscriminatedUnionSchema_NoDiscriminator(t *testing.T) {
	// When no discriminator tag targets this field, discriminator should be nil
	schema := buildDiscriminatedUnionSchema("event", "TypeA,TypeB", map[string]string{})

	if len(schema.OneOf) != 2 {
		t.Fatalf("expected 2 oneOf entries, got %d", len(schema.OneOf))
	}
	if schema.Discriminator != nil {
		t.Error("expected discriminator to be nil when no discriminator tag targets this field")
	}
}

func TestBuildDiscriminatedUnionSchema_EmptyTypeNames(t *testing.T) {
	// Empty and whitespace-only type names should be skipped
	schema := buildDiscriminatedUnionSchema("event", "TypeA, ,TypeB, ", map[string]string{})

	if len(schema.OneOf) != 2 {
		t.Fatalf("expected 2 oneOf entries (empty names skipped), got %d", len(schema.OneOf))
	}
	if schema.OneOf[0].Ref != "#/components/schemas/TypeA" {
		t.Errorf("expected ref to TypeA, got %q", schema.OneOf[0].Ref)
	}
	if schema.OneOf[1].Ref != "#/components/schemas/TypeB" {
		t.Errorf("expected ref to TypeB, got %q", schema.OneOf[1].Ref)
	}
}

func TestMetadataToSchema_DiscriminatedUnion(t *testing.T) {
	meta := sentinel.Metadata{
		TypeName: "Notification",
		Fields: []sentinel.FieldMetadata{
			{
				Name: "Type",
				Type: "string",
				Tags: map[string]string{
					"json":          "type",
					"discriminator": "event",
				},
			},
			{
				Name: "Event",
				Type: "any",
				Tags: map[string]string{
					"json":         "event",
					"discriminate": "IngestCompletedEvent,IngestFailedEvent",
				},
			},
		},
	}

	schema := metadataToSchema(meta)

	t.Run("type field is normal string", func(t *testing.T) {
		typeProp := schema.Properties["type"]
		if typeProp == nil {
			t.Fatal("expected 'type' property")
		}
		if typeProp.Type == nil || typeProp.Type.String() != "string" {
			t.Errorf("expected string type, got %v", typeProp.Type)
		}
	})

	t.Run("event field is oneOf with discriminator", func(t *testing.T) {
		eventProp := schema.Properties["event"]
		if eventProp == nil {
			t.Fatal("expected 'event' property")
		}
		if len(eventProp.OneOf) != 2 {
			t.Fatalf("expected 2 oneOf entries, got %d", len(eventProp.OneOf))
		}
		if eventProp.OneOf[0].Ref != "#/components/schemas/IngestCompletedEvent" {
			t.Errorf("expected ref to IngestCompletedEvent, got %q", eventProp.OneOf[0].Ref)
		}
		if eventProp.Discriminator == nil {
			t.Fatal("expected discriminator")
		}
		if eventProp.Discriminator.PropertyName != "type" {
			t.Errorf("expected propertyName 'type', got %q", eventProp.Discriminator.PropertyName)
		}
	})

	t.Run("both fields required", func(t *testing.T) {
		if len(schema.Required) != 2 {
			t.Errorf("expected 2 required fields, got %v", schema.Required)
		}
	})
}

func TestResolveTypeName(t *testing.T) {
	// Scan a type so it's in sentinel's cache
	_ = NewModel[testModelType]()

	meta, found := resolveTypeName("testModelType")
	if !found {
		t.Fatal("expected to find testModelType via resolveTypeName")
	}
	if meta.TypeName != "testModelType" {
		t.Errorf("expected type name 'testModelType', got %q", meta.TypeName)
	}
}

func TestResolveTypeName_NotFound(t *testing.T) {
	_, found := resolveTypeName("NonExistentType12345")
	if found {
		t.Error("expected resolveTypeName to return false for non-existent type")
	}
}

func TestGenerateOpenAPI_DiscriminatedUnionAutoDiscovery(t *testing.T) {
	// Variant types are scanned by sentinel (via NewModel) but NOT registered
	// with WithModels. They should still be discovered via resolveTypeName
	// when collectSchemas processes the discriminate tag on the parent type.
	type AutoEventA struct {
		Status string `json:"status"`
	}
	type AutoEventB struct {
		Reason string `json:"reason"`
	}
	type AutoParent struct {
		Kind    string `json:"kind" discriminator:"payload"`
		Payload any    `json:"payload" discriminate:"AutoEventA,AutoEventB"`
	}

	// Scan types into sentinel cache without WithModels
	_ = NewModel[AutoEventA]()
	_ = NewModel[AutoEventB]()

	engine := newTestEngine()
	handler := NewHandler[NoBody, AutoParent](
		"get-auto",
		"GET",
		"/auto",
		func(req *Request[NoBody]) (AutoParent, error) {
			return AutoParent{}, nil
		},
	)
	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	if _, exists := spec.Components.Schemas["AutoEventA"]; !exists {
		t.Error("expected AutoEventA discovered via resolveTypeName")
	}
	if _, exists := spec.Components.Schemas["AutoEventB"]; !exists {
		t.Error("expected AutoEventB discovered via resolveTypeName")
	}
}

func TestGenerateOpenAPI_DiscriminatedUnion(t *testing.T) {
	type IngestCompletedEvent struct {
		DocumentID   string `json:"document_id"`
		DocumentName string `json:"document_name"`
	}

	type IngestFailedEvent struct {
		DocumentID string `json:"document_id"`
		Error      string `json:"error"`
	}

	type Notification struct {
		Type  string `json:"type" discriminator:"event"`
		Event any    `json:"event" discriminate:"IngestCompletedEvent,IngestFailedEvent"`
	}

	engine := newTestEngine()
	engine.WithModels(
		NewModel[IngestCompletedEvent](),
		NewModel[IngestFailedEvent](),
	)

	handler := NewHandler[NoBody, Notification](
		"get-notification",
		"GET",
		"/notifications/{id}",
		func(req *Request[NoBody]) (Notification, error) {
			return Notification{}, nil
		},
	).WithPathParams("id")

	engine.WithHandlers(handler)
	spec := engine.GenerateOpenAPI(nil)

	t.Run("variant schemas registered", func(t *testing.T) {
		if _, exists := spec.Components.Schemas["IngestCompletedEvent"]; !exists {
			t.Error("expected IngestCompletedEvent in component schemas")
		}
		if _, exists := spec.Components.Schemas["IngestFailedEvent"]; !exists {
			t.Error("expected IngestFailedEvent in component schemas")
		}
	})

	t.Run("notification schema has oneOf", func(t *testing.T) {
		notif := spec.Components.Schemas["Notification"]
		if notif == nil {
			t.Fatal("expected Notification schema")
		}
		eventProp := notif.Properties["event"]
		if eventProp == nil {
			t.Fatal("expected 'event' property on Notification")
		}
		if len(eventProp.OneOf) != 2 {
			t.Fatalf("expected 2 oneOf entries, got %d", len(eventProp.OneOf))
		}
	})

	t.Run("discriminator set correctly", func(t *testing.T) {
		notif := spec.Components.Schemas["Notification"]
		eventProp := notif.Properties["event"]
		if eventProp.Discriminator == nil {
			t.Fatal("expected discriminator on event property")
		}
		if eventProp.Discriminator.PropertyName != "type" {
			t.Errorf("expected propertyName 'type', got %q", eventProp.Discriminator.PropertyName)
		}
		if len(eventProp.Discriminator.Mapping) != 2 {
			t.Errorf("expected 2 mapping entries, got %d", len(eventProp.Discriminator.Mapping))
		}
	})
}

// TestGenerateOpenAPI_RedirectHandler verifies the spec matches what the
// runtime write path actually does for redirect handlers: a 3xx response
// with a Location header, no body, and no marker-type schemas in components.
// Both sides read the same ResponseContract, so this cannot drift.
func TestGenerateOpenAPI_RedirectHandler(t *testing.T) {
	engine := NewEngine()
	handler := GET[NoBody, Redirect]("/old-path",
		func(_ *Request[NoBody]) (Redirect, error) {
			return Redirect{URL: "/new"}, nil
		})
	engine.WithHandlers(handler)

	spec := engine.GenerateOpenAPI(nil)

	op := spec.Paths["/old-path"].Get
	if op == nil {
		t.Fatal("expected GET operation")
	}

	resp, ok := op.Responses["302"]
	if !ok {
		t.Fatalf("expected 302 response, got keys: %v", responseKeys(op.Responses))
	}
	if resp.Content != nil {
		t.Error("redirect response must not declare a content schema")
	}
	if resp.Headers["Location"] == nil {
		t.Error("redirect response must declare the Location header")
	}
	if _, exists := op.Responses["200"]; exists {
		t.Error("redirect handler must not declare a 200 response")
	}

	// Marker types must not pollute component schemas.
	if _, exists := spec.Components.Schemas["Redirect"]; exists {
		t.Error("Redirect marker type must not appear in components")
	}
	if _, exists := spec.Components.Schemas["Header"]; exists {
		t.Error("dangling Header schema must not appear in components")
	}
}

func responseKeys(m map[string]openapi.Response) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestGenerateOpenAPI_RedirectStatusOverride verifies WithSuccessStatus
// changes the documented redirect status via the response contract.
func TestGenerateOpenAPI_RedirectStatusOverride(t *testing.T) {
	engine := NewEngine()
	handler := GET[NoBody, Redirect]("/moved",
		func(_ *Request[NoBody]) (Redirect, error) {
			return Redirect{URL: "/new"}, nil
		}).WithSuccessStatus(http.StatusMovedPermanently)
	engine.WithHandlers(handler)

	spec := engine.GenerateOpenAPI(nil)
	op := spec.Paths["/moved"].Get
	if op == nil {
		t.Fatal("expected GET operation")
	}
	if _, ok := op.Responses["301"]; !ok {
		t.Errorf("expected 301 response, got keys: %v", responseKeys(op.Responses))
	}
}
