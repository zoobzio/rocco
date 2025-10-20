package rocco

import (
	"testing"
)

func TestOpenAPI_Structure(t *testing.T) {
	spec := &OpenAPI{
		OpenAPI: "3.0.3",
		Info: Info{
			Title:       "Test API",
			Version:     "1.0.0",
			Description: "A test API",
		},
		Paths: make(map[string]PathItem),
		Components: &Components{
			Schemas: make(map[string]*Schema),
		},
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("expected openapi '3.0.3', got %q", spec.OpenAPI)
	}
	if spec.Info.Title != "Test API" {
		t.Errorf("expected title 'Test API', got %q", spec.Info.Title)
	}
	if spec.Paths == nil {
		t.Error("paths should not be nil")
	}
	if spec.Components == nil {
		t.Error("components should not be nil")
	}
}

func TestInfo(t *testing.T) {
	info := Info{
		Title:       "My API",
		Version:     "2.0.0",
		Description: "API Description",
		Contact: &Contact{
			Name:  "Support",
			URL:   "https://example.com",
			Email: "support@example.com",
		},
	}

	if info.Title != "My API" {
		t.Errorf("expected title 'My API', got %q", info.Title)
	}
	if info.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", info.Version)
	}
	if info.Contact == nil {
		t.Fatal("contact should not be nil")
	}
	if info.Contact.Email != "support@example.com" {
		t.Errorf("expected email 'support@example.com', got %q", info.Contact.Email)
	}
}

func TestServer(t *testing.T) {
	server := Server{
		URL:         "https://api.example.com",
		Description: "Production server",
	}

	if server.URL != "https://api.example.com" {
		t.Errorf("expected url 'https://api.example.com', got %q", server.URL)
	}
	if server.Description != "Production server" {
		t.Errorf("expected description 'Production server', got %q", server.Description)
	}
}

func TestPathItem(t *testing.T) {
	pathItem := PathItem{
		Summary:     "User operations",
		Description: "Operations for user management",
		Get: &Operation{
			OperationID: "getUser",
			Summary:     "Get user",
		},
		Post: &Operation{
			OperationID: "createUser",
			Summary:     "Create user",
		},
	}

	if pathItem.Summary != "User operations" {
		t.Errorf("expected summary 'User operations', got %q", pathItem.Summary)
	}
	if pathItem.Get == nil {
		t.Fatal("GET operation should not be nil")
	}
	if pathItem.Get.OperationID != "getUser" {
		t.Errorf("expected operation ID 'getUser', got %q", pathItem.Get.OperationID)
	}
}

func TestOperation(t *testing.T) {
	operation := Operation{
		Tags:        []string{"users"},
		Summary:     "Get user by ID",
		Description: "Returns a single user",
		OperationID: "getUserById",
		Parameters: []Parameter{
			{
				Name:     "id",
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: "string"},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Success",
			},
		},
	}

	if len(operation.Tags) != 1 || operation.Tags[0] != "users" {
		t.Errorf("expected tags ['users'], got %v", operation.Tags)
	}
	if operation.OperationID != "getUserById" {
		t.Errorf("expected operation ID 'getUserById', got %q", operation.OperationID)
	}
	if len(operation.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(operation.Parameters))
	}
	if len(operation.Responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(operation.Responses))
	}
}

func TestParameter(t *testing.T) {
	param := Parameter{
		Name:        "userId",
		In:          "path",
		Description: "User ID",
		Required:    true,
		Schema:      &Schema{Type: "string"},
	}

	if param.Name != "userId" {
		t.Errorf("expected name 'userId', got %q", param.Name)
	}
	if param.In != "path" {
		t.Errorf("expected in 'path', got %q", param.In)
	}
	if !param.Required {
		t.Error("expected required to be true")
	}
	if param.Schema.Type != "string" {
		t.Errorf("expected schema type 'string', got %q", param.Schema.Type)
	}
}

func TestRequestBody(t *testing.T) {
	reqBody := RequestBody{
		Description: "User object",
		Required:    true,
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Ref: "#/components/schemas/User"},
			},
		},
	}

	if reqBody.Description != "User object" {
		t.Errorf("expected description 'User object', got %q", reqBody.Description)
	}
	if !reqBody.Required {
		t.Error("expected required to be true")
	}
	if len(reqBody.Content) != 1 {
		t.Errorf("expected 1 content type, got %d", len(reqBody.Content))
	}
}

func TestResponse(t *testing.T) {
	response := Response{
		Description: "Successful operation",
		Content: map[string]MediaType{
			"application/json": {
				Schema: &Schema{Type: "object"},
			},
		},
	}

	if response.Description != "Successful operation" {
		t.Errorf("expected description 'Successful operation', got %q", response.Description)
	}
	if len(response.Content) != 1 {
		t.Errorf("expected 1 content type, got %d", len(response.Content))
	}
}

func TestSchema_BasicTypes(t *testing.T) {
	tests := []struct {
		name   string
		schema Schema
	}{
		{"string", Schema{Type: "string"}},
		{"integer", Schema{Type: "integer"}},
		{"number", Schema{Type: "number"}},
		{"boolean", Schema{Type: "boolean"}},
		{"array", Schema{Type: "array", Items: &Schema{Type: "string"}}},
		{"object", Schema{Type: "object", Properties: map[string]*Schema{
			"name": {Type: "string"},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.schema.Type != tt.name {
				t.Errorf("expected type %q, got %q", tt.name, tt.schema.Type)
			}
		})
	}
}

func TestSchema_WithRef(t *testing.T) {
	schema := Schema{
		Ref: "#/components/schemas/User",
	}

	if schema.Ref != "#/components/schemas/User" {
		t.Errorf("expected ref '#/components/schemas/User', got %q", schema.Ref)
	}
}

func TestSchema_WithRequired(t *testing.T) {
	schema := Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"name":  {Type: "string"},
			"email": {Type: "string"},
		},
		Required: []string{"name", "email"},
	}

	if len(schema.Required) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(schema.Required))
	}
}

func TestSchema_WithFormat(t *testing.T) {
	schema := Schema{
		Type:   "string",
		Format: "date-time",
	}

	if schema.Format != "date-time" {
		t.Errorf("expected format 'date-time', got %q", schema.Format)
	}
}

func TestComponents(t *testing.T) {
	components := Components{
		Schemas: map[string]*Schema{
			"User": {
				Type: "object",
				Properties: map[string]*Schema{
					"id":   {Type: "integer"},
					"name": {Type: "string"},
				},
			},
		},
		Responses: map[string]*Response{
			"NotFound": {
				Description: "Resource not found",
			},
		},
	}

	if len(components.Schemas) != 1 {
		t.Errorf("expected 1 schema, got %d", len(components.Schemas))
	}
	if len(components.Responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(components.Responses))
	}
}

func TestTag(t *testing.T) {
	tag := Tag{
		Name:        "users",
		Description: "User management operations",
	}

	if tag.Name != "users" {
		t.Errorf("expected name 'users', got %q", tag.Name)
	}
	if tag.Description != "User management operations" {
		t.Errorf("expected description 'User management operations', got %q", tag.Description)
	}
}

func TestMediaType(t *testing.T) {
	mediaType := MediaType{
		Schema: &Schema{Type: "object"},
		Example: map[string]any{
			"name": "John",
			"age":  30,
		},
	}

	if mediaType.Schema.Type != "object" {
		t.Errorf("expected schema type 'object', got %q", mediaType.Schema.Type)
	}
	if mediaType.Example == nil {
		t.Error("expected example to be set")
	}
}
