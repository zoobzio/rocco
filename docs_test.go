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

	engine.Register(handler1, handler2)

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

	engine.Register(handler)

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

	engine.Register(handler)

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
