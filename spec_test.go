package rocco

import (
	"encoding/json"
	"testing"

	"github.com/zoobz-io/openapi"
)

func TestHandlerSpec_Serialization(t *testing.T) {
	spec := HandlerSpec{
		Name:           "create-user",
		Method:         "POST",
		Path:           "/users",
		Summary:        "Create a user",
		Description:    "Creates a new user account",
		Tags:           []string{"users"},
		PathParams:     []string{},
		QueryParams:    []string{"page", "limit"},
		InputTypeName:  "CreateUserInput",
		OutputTypeName: "UserOutput",
		Response:       ResponseContract{Kind: BodyEncoded, Status: 201},
		RequiresAuth:   true,
		ScopeGroups:    [][]string{{"write:users"}},
		RoleGroups:     [][]string{{"admin", "moderator"}},
	}

	// Test JSON serialization round-trip.
	// Note: UsageLimits.ThresholdFunc is not serializable, but we're testing without it.
	data, err := json.Marshal(spec) //nolint:staticcheck // Testing serializable fields only
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded HandlerSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Name != spec.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, spec.Name)
	}
	if decoded.Method != spec.Method {
		t.Errorf("Method = %q, want %q", decoded.Method, spec.Method)
	}
	if decoded.Path != spec.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, spec.Path)
	}
	if decoded.Response.Status != spec.Response.Status {
		t.Errorf("SuccessStatus = %d, want %d", decoded.Response.Status, spec.Response.Status)
	}
	if decoded.RequiresAuth != spec.RequiresAuth {
		t.Errorf("RequiresAuth = %v, want %v", decoded.RequiresAuth, spec.RequiresAuth)
	}
}

func TestHandlerSpec_StreamFlag(t *testing.T) {
	spec := HandlerSpec{
		Name:     "event-stream",
		Method:   "GET",
		Path:     "/events",
		Response: ResponseContract{Kind: BodyStream, Status: 200},
	}

	data, err := json.Marshal(spec) //nolint:staticcheck // Testing serializable fields only
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded HandlerSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Response.Kind != BodyStream {
		t.Error("Response.Kind != BodyStream after round-trip")
	}
}

func TestEngineSpec_Serialization(t *testing.T) {
	spec := EngineSpec{
		Info: openapi.Info{
			Title:       "Test API",
			Version:     "1.0.0",
			Description: "Test API description",
		},
		Tags: []openapi.Tag{
			{Name: "users", Description: "User operations"},
		},
		TagGroups: []openapi.TagGroup{
			{Name: "Account", Tags: []string{"users", "auth"}},
		},
		Servers: []openapi.Server{
			{URL: "https://api.example.com"},
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded EngineSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Info.Title != spec.Info.Title {
		t.Errorf("Info.Title = %q, want %q", decoded.Info.Title, spec.Info.Title)
	}
	if decoded.Info.Version != spec.Info.Version {
		t.Errorf("Info.Version = %q, want %q", decoded.Info.Version, spec.Info.Version)
	}
	if len(decoded.Tags) != len(spec.Tags) {
		t.Errorf("Tags length = %d, want %d", len(decoded.Tags), len(spec.Tags))
	}
	if len(decoded.TagGroups) != 1 {
		t.Errorf("TagGroups length = %d, want 1", len(decoded.TagGroups))
	}
	if decoded.TagGroups[0].Name != "Account" {
		t.Errorf("TagGroups[0].Name = %q, want %q", decoded.TagGroups[0].Name, "Account")
	}
	if len(decoded.TagGroups[0].Tags) != 2 {
		t.Errorf("TagGroups[0].Tags length = %d, want 2", len(decoded.TagGroups[0].Tags))
	}
	if len(decoded.Servers) != len(spec.Servers) {
		t.Errorf("Servers length = %d, want %d", len(decoded.Servers), len(spec.Servers))
	}
}

func TestDefaultEngineSpec(t *testing.T) {
	spec := DefaultEngineSpec()

	if spec == nil {
		t.Fatal("DefaultEngineSpec() returned nil")
	}

	if spec.Info.Title != "API" {
		t.Errorf("Info.Title = %q, want %q", spec.Info.Title, "API")
	}
	if spec.Info.Version != "1.0.0" {
		t.Errorf("Info.Version = %q, want %q", spec.Info.Version, "1.0.0")
	}
	if spec.Tags == nil {
		t.Error("Tags is nil, want empty slice")
	}
	if spec.Servers == nil {
		t.Error("Servers is nil, want empty slice")
	}
}

func TestHandlerSpec_MediaTypes(t *testing.T) {
	spec := HandlerSpec{
		Name:   "upload",
		Method: "POST",
		Path:   "/upload",
		Request: RequestContract{
			Kind:       BodyEncoded,
			MediaTypes: []string{"multipart/form-data"},
		},
		Response: ResponseContract{
			Kind:       BodyEncoded,
			Status:     200,
			MediaTypes: []string{"application/json"},
		},
	}

	data, err := json.Marshal(spec) //nolint:staticcheck // Testing serializable fields only
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded HandlerSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got := primaryMediaType(decoded.Request.MediaTypes); got != "multipart/form-data" {
		t.Errorf("Request media type = %q, want %q", got, "multipart/form-data")
	}
	if got := primaryMediaType(decoded.Response.MediaTypes); got != "application/json" {
		t.Errorf("Response media type = %q, want %q", got, "application/json")
	}
}

func TestPrimaryMediaType_EmptyFallsBackToJSON(t *testing.T) {
	if got := primaryMediaType(nil); got != ContentTypeJSON {
		t.Errorf("primaryMediaType(nil) = %q, want %q", got, ContentTypeJSON)
	}
	if got := primaryMediaType([]string{"image/png", "application/pdf"}); got != "image/png" {
		t.Errorf("primaryMediaType = %q, want first entry %q", got, "image/png")
	}
}

func TestHandlerSpec_UsageLimits(t *testing.T) {
	limitFunc := func(_ Identity) int { return 100 }

	spec := HandlerSpec{
		Name:   "limited",
		Method: "GET",
		Path:   "/limited",
		UsageLimits: []UsageLimit{
			{Key: "requests_today", ThresholdFunc: limitFunc},
		},
	}

	if len(spec.UsageLimits) != 1 {
		t.Errorf("UsageLimits length = %d, want 1", len(spec.UsageLimits))
	}
	if spec.UsageLimits[0].Key != "requests_today" {
		t.Errorf("UsageLimits[0].Key = %q, want %q", spec.UsageLimits[0].Key, "requests_today")
	}
	if spec.UsageLimits[0].ThresholdFunc(nil) != 100 {
		t.Errorf("UsageLimits[0].ThresholdFunc() = %d, want 100", spec.UsageLimits[0].ThresholdFunc(nil))
	}
}
