package rocco

import (
	"testing"

	"github.com/zoobzio/sentinel"
)

func TestMetadataToSchema(t *testing.T) {
	meta := sentinel.ModelMetadata{
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

	if schema.Type != "object" {
		t.Errorf("expected type 'object', got %q", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(schema.Properties))
	}
	if schema.Properties["name"].Type != "string" {
		t.Errorf("expected name type 'string', got %q", schema.Properties["name"].Type)
	}
	if schema.Properties["count"].Type != "integer" {
		t.Errorf("expected count type 'integer', got %q", schema.Properties["count"].Type)
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
			if schema.Type != tt.wantType {
				t.Errorf("expected type %q, got %q", tt.wantType, schema.Type)
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

func TestGoTypeToSchema_ComplexType(t *testing.T) {
	schema := goTypeToSchema("github.com/user/pkg.CustomType")

	if schema.Ref != "#/components/schemas/CustomType" {
		t.Errorf("expected ref '#/components/schemas/CustomType', got %q", schema.Ref)
	}
}

func TestSchemaName(t *testing.T) {
	meta := sentinel.ModelMetadata{
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

func TestIsNoBodySchema(t *testing.T) {
	tests := []struct {
		name   string
		schema *Schema
		want   bool
	}{
		{
			"nil schema",
			nil,
			false,
		},
		{
			"empty object",
			&Schema{Type: "object", Properties: map[string]*Schema{}},
			true,
		},
		{
			"object with properties",
			&Schema{Type: "object", Properties: map[string]*Schema{
				"field": {Type: "string"},
			}},
			false,
		},
		{
			"non-object",
			&Schema{Type: "string"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoBodySchema(tt.schema)
			if result != tt.want {
				t.Errorf("expected %v, got %v", tt.want, result)
			}
		})
	}
}

func TestSetOperationForMethod(t *testing.T) {
	tests := []struct {
		method string
		check  func(*PathItem) bool
	}{
		{"GET", func(pi *PathItem) bool { return pi.Get != nil }},
		{"POST", func(pi *PathItem) bool { return pi.Post != nil }},
		{"PUT", func(pi *PathItem) bool { return pi.Put != nil }},
		{"DELETE", func(pi *PathItem) bool { return pi.Delete != nil }},
		{"PATCH", func(pi *PathItem) bool { return pi.Patch != nil }},
		{"OPTIONS", func(pi *PathItem) bool { return pi.Options != nil }},
		{"HEAD", func(pi *PathItem) bool { return pi.Head != nil }},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			pathItem := &PathItem{}
			operation := &Operation{OperationID: "test"}

			setOperationForMethod(pathItem, tt.method, operation)

			if !tt.check(pathItem) {
				t.Errorf("operation not set for method %s", tt.method)
			}
		})
	}
}

func TestGenerateOpenAPI(t *testing.T) {
	engine := NewEngine(nil)

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
	).WithSummary("Create test").WithTags("test").WithErrorCodes(400, 404)

	engine.WithHandlers(handler1, handler2)

	// Generate OpenAPI spec
	info := Info{
		Title:       "Test API",
		Version:     "1.0.0",
		Description: "Test API description",
	}
	spec := engine.GenerateOpenAPI(info)

	// Check spec structure
	if spec.OpenAPI != "3.0.3" {
		t.Errorf("expected OpenAPI version '3.0.3', got %q", spec.OpenAPI)
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
	if len(spec.Components.Responses) == 0 {
		t.Error("expected responses in components")
	}

	// Check standard error response
	if _, exists := spec.Components.Schemas["ErrorResponse"]; !exists {
		t.Error("expected ErrorResponse schema")
	}
}

func TestGenerateOpenAPI_PathParams(t *testing.T) {
	engine := NewEngine(nil)

	handler := NewHandler[NoBody, testOutput](
		"get-user",
		"GET",
		"/users/{id}",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithPathParams("id")

	engine.WithHandlers(handler)

	spec := engine.GenerateOpenAPI(Info{Title: "Test", Version: "1.0.0"})

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
	engine := NewEngine(nil)

	handler := NewHandler[NoBody, testOutput](
		"list-users",
		"GET",
		"/users",
		func(req *Request[NoBody]) (testOutput, error) {
			return testOutput{}, nil
		},
	).WithQueryParams("page", "limit")

	engine.WithHandlers(handler)

	spec := engine.GenerateOpenAPI(Info{Title: "Test", Version: "1.0.0"})

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

	schema := &Schema{Type: "string"}
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

	schema := &Schema{Type: "string"}
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

			schema := &Schema{Type: tt.schemaType}
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

			schema := &Schema{Type: tt.schemaType}
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

	schema := &Schema{Type: "integer"}
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

	schema := &Schema{Type: "string"}
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

	schema := &Schema{Type: "array"}
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

	schema := &Schema{Type: "string"}
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
	meta := sentinel.ModelMetadata{
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

	schema := &Schema{Type: "integer"}
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

	schema := &Schema{Type: "number"}
	applyOpenAPITags(schema, field)

	if schema.Minimum == nil || *schema.Minimum != 0 {
		t.Errorf("expected minimum 0 from gt, got %v", schema.Minimum)
	}
	if schema.ExclusiveMinimum == nil || *schema.ExclusiveMinimum != true {
		t.Errorf("expected exclusiveMinimum true from gt, got %v", schema.ExclusiveMinimum)
	}
	if schema.Maximum == nil || *schema.Maximum != 1 {
		t.Errorf("expected maximum 1 from lt, got %v", schema.Maximum)
	}
	if schema.ExclusiveMaximum == nil || *schema.ExclusiveMaximum != true {
		t.Errorf("expected exclusiveMaximum true from lt, got %v", schema.ExclusiveMaximum)
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

	schema := &Schema{Type: "string"}
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

	schema := &Schema{Type: "string"}
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

			schema := &Schema{Type: "string"}
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

			schema := &Schema{Type: "string"}
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

	schema := &Schema{Type: "string"}
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
