package rocco

import (
	"testing"

	"github.com/zoobzio/openapi"
	"github.com/zoobzio/sentinel"
)

func TestParseValidateTag_NumericConstraints(t *testing.T) {
	// OpenAPI 3.1.0: exclusiveMinimum/Maximum are the actual bound values, not booleans
	tests := []struct {
		name        string
		validateTag string
		goType      string
		wantMin     *float64
		wantMax     *float64
		wantExclMin *float64 // OpenAPI 3.1.0: exclusive bounds are float values
		wantExclMax *float64 // OpenAPI 3.1.0: exclusive bounds are float values
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
		{
			name:        "gte constraint",
			validateTag: "gte=10",
			goType:      "int",
			wantMin:     float64Ptr(10),
		},
		{
			name:        "lte constraint",
			validateTag: "lte=20",
			goType:      "int",
			wantMax:     float64Ptr(20),
		},
		{
			name:        "gt constraint (exclusive)",
			validateTag: "gt=0",
			goType:      "int",
			wantExclMin: float64Ptr(0), // OpenAPI 3.1.0: exclusiveMinimum is the value itself
		},
		{
			name:        "lt constraint (exclusive)",
			validateTag: "lt=100",
			goType:      "int",
			wantExclMax: float64Ptr(100), // OpenAPI 3.1.0: exclusiveMaximum is the value itself
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

			if tt.wantExclMin != nil {
				exclMin := constraints["exclusiveMinimum"].(*float64)
				if *exclMin != *tt.wantExclMin {
					t.Errorf("exclusiveMinimum = %v, want %v", *exclMin, *tt.wantExclMin)
				}
			}

			if tt.wantExclMax != nil {
				exclMax := constraints["exclusiveMaximum"].(*float64)
				if *exclMax != *tt.wantExclMax {
					t.Errorf("exclusiveMaximum = %v, want %v", *exclMax, *tt.wantExclMax)
				}
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
		{
			name:        "email format",
			validateTag: "email",
			goType:      "string",
			wantFormat:  "email",
		},
		{
			name:        "url format",
			validateTag: "url",
			goType:      "string",
			wantFormat:  "uri",
		},
		{
			name:        "uuid format",
			validateTag: "uuid",
			goType:      "string",
			wantFormat:  "uuid",
		},
		{
			name:        "uuid4 format",
			validateTag: "uuid4",
			goType:      "string",
			wantFormat:  "uuid",
		},
		{
			name:        "datetime format",
			validateTag: "datetime",
			goType:      "string",
			wantFormat:  "date-time",
		},
		{
			name:        "ipv4 format",
			validateTag: "ipv4",
			goType:      "string",
			wantFormat:  "ipv4",
		},
		{
			name:        "ipv6 format",
			validateTag: "ipv6",
			goType:      "string",
			wantFormat:  "ipv6",
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
	// Test multiple constraints in a single tag
	constraints := parseValidateTag("min=1,max=100,required", "int")

	if minVal := constraints["minimum"].(*float64); *minVal != 1 {
		t.Errorf("minimum = %v, want 1", *minVal)
	}

	if maxVal := constraints["maximum"].(*float64); *maxVal != 100 {
		t.Errorf("maximum = %v, want 100", *maxVal)
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

func TestApplyOpenAPITags_ValidateTag(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Age",
		Type: "int",
		Tags: map[string]string{
			"validate":    "min=0,max=120",
			"description": "User's age in years",
			"example":     "25",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("integer")}
	applyOpenAPITags(schema, field)

	// Check validate-derived constraints
	if schema.Minimum == nil || *schema.Minimum != 0 {
		t.Errorf("minimum = %v, want 0", schema.Minimum)
	}
	if schema.Maximum == nil || *schema.Maximum != 120 {
		t.Errorf("maximum = %v, want 120", schema.Maximum)
	}

	// Check documentation tags
	if schema.Description != "User's age in years" {
		t.Errorf("description = %q, want 'User's age in years'", schema.Description)
	}
	if schema.Example != 25 {
		t.Errorf("example = %v, want 25", schema.Example)
	}
}

func TestApplyOpenAPITags_ValidateEmail(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Email",
		Type: "string",
		Tags: map[string]string{
			"validate":    "email,min=5,max=100",
			"description": "User email address",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Format != "email" {
		t.Errorf("format = %q, want 'email'", schema.Format)
	}
	if schema.MinLength == nil || *schema.MinLength != 5 {
		t.Errorf("minLength = %v, want 5", schema.MinLength)
	}
	if schema.MaxLength == nil || *schema.MaxLength != 100 {
		t.Errorf("maxLength = %v, want 100", schema.MaxLength)
	}
}

func TestApplyOpenAPITags_ValidateArray(t *testing.T) {
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
		t.Errorf("minItems = %v, want 5", schema.MinItems)
	}
	if schema.MaxItems == nil || *schema.MaxItems != 5 {
		t.Errorf("maxItems = %v, want 5", schema.MaxItems)
	}
	if schema.UniqueItems == nil || *schema.UniqueItems != true {
		t.Errorf("uniqueItems = %v, want true", schema.UniqueItems)
	}
}

func TestApplyOpenAPITags_ValidateEnum(t *testing.T) {
	field := sentinel.FieldMetadata{
		Name: "Status",
		Type: "string",
		Tags: map[string]string{
			"validate": "oneof=active inactive pending",
		},
	}

	schema := &openapi.Schema{Type: openapi.NewSchemaType("string")}
	applyOpenAPITags(schema, field)

	if schema.Enum == nil {
		t.Fatal("expected enum to be set")
	}
	if len(schema.Enum) != 3 {
		t.Fatalf("enum length = %d, want 3", len(schema.Enum))
	}

	wantEnum := []any{"active", "inactive", "pending"}
	for i, want := range wantEnum {
		if schema.Enum[i] != want {
			t.Errorf("enum[%d] = %v, want %v", i, schema.Enum[i], want)
		}
	}
}
