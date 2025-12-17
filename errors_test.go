package rocco

import (
	"errors"
	"testing"
)

func TestError_Interface(t *testing.T) {
	// Verify Error implements error and ErrorDefinition interfaces
	var _ error = &Error[NoDetails]{}
	var _ error = ErrNotFound
	var _ ErrorDefinition = ErrNotFound
}

func TestError_Accessors(t *testing.T) {
	err := NewError[NoDetails]("TEST_ERROR", 418, "test message")

	if err.Code() != "TEST_ERROR" {
		t.Errorf("expected code TEST_ERROR, got %s", err.Code())
	}
	if err.Status() != 418 {
		t.Errorf("expected status 418, got %d", err.Status())
	}
	if err.Message() != "test message" {
		t.Errorf("expected message 'test message', got %s", err.Message())
	}
	if err.Error() != "test message" {
		t.Errorf("expected Error() to return message, got %s", err.Error())
	}
	if err.Unwrap() != nil {
		t.Errorf("expected nil cause, got %v", err.Unwrap())
	}
}

func TestError_WithMessage(t *testing.T) {
	original := NewError[NoDetails]("TEST", 400, "original")
	modified := original.WithMessage("modified")

	// Original should be unchanged (immutable)
	if original.Message() != "original" {
		t.Error("original error was mutated")
	}

	// Modified should have new message
	if modified.Message() != "modified" {
		t.Errorf("expected modified message, got %s", modified.Message())
	}

	// Code and status should be preserved
	if modified.Code() != "TEST" {
		t.Errorf("code was not preserved")
	}
	if modified.Status() != 400 {
		t.Errorf("status was not preserved")
	}
}

func TestError_WithDetails(t *testing.T) {
	type TestDetails struct {
		UserID string `json:"user_id"`
	}

	original := NewError[TestDetails]("TEST", 400, "test")
	details := TestDetails{UserID: "123"}
	modified := original.WithDetails(details)

	// Original should have zero-value details
	if original.Details().UserID != "" {
		t.Error("original error was mutated")
	}

	// Modified should have details
	if modified.Details().UserID != "123" {
		t.Errorf("expected user_id 123, got %s", modified.Details().UserID)
	}
}

func TestError_WithCause(t *testing.T) {
	cause := errors.New("underlying error")
	original := NewError[NoDetails]("TEST", 500, "test")
	modified := original.WithCause(cause)

	// Original should have no cause
	if original.Unwrap() != nil {
		t.Error("original error was mutated")
	}

	// Modified should have cause
	if modified.Unwrap() != cause {
		t.Error("cause was not set correctly")
	}
}

func TestError_Chaining(t *testing.T) {
	cause := errors.New("db error")
	err := ErrNotFound.
		WithMessage("user not found").
		WithDetails(NotFoundDetails{Resource: "user"}).
		WithCause(cause)

	if err.Code() != "NOT_FOUND" {
		t.Errorf("code not preserved: %s", err.Code())
	}
	if err.Status() != 404 {
		t.Errorf("status not preserved: %d", err.Status())
	}
	if err.Message() != "user not found" {
		t.Errorf("message not set: %s", err.Message())
	}
	if err.Details().Resource != "user" {
		t.Error("details not set")
	}
	if err.Unwrap() != cause {
		t.Error("cause not set")
	}
}

func TestError_Is(t *testing.T) {
	// Same code should match
	err1 := NewError[NoDetails]("NOT_FOUND", 404, "not found")
	err2 := NewError[NoDetails]("NOT_FOUND", 404, "different message")
	if !errors.Is(err1, err2) {
		t.Error("errors with same code should match")
	}

	// Different code should not match
	err3 := NewError[NoDetails]("BAD_REQUEST", 400, "bad request")
	if errors.Is(err1, err3) {
		t.Error("errors with different codes should not match")
	}

	// WithMessage should preserve Is behavior
	err4 := ErrNotFound.WithMessage("custom message")
	if !errors.Is(err4, ErrNotFound) {
		t.Error("WithMessage should preserve Is behavior")
	}

	// WithDetails should preserve Is behavior
	err5 := ErrNotFound.WithDetails(NotFoundDetails{Resource: "user"})
	if !errors.Is(err5, ErrNotFound) {
		t.Error("WithDetails should preserve Is behavior")
	}
}

func TestError_Is_NonErrorDefinition(t *testing.T) {
	// Test that Is returns false for non-ErrorDefinition targets
	roccoErr := NewError[NoDetails]("TEST", 400, "test")
	plainErr := errors.New("plain error")

	// Is() should return false when target is not ErrorDefinition
	if roccoErr.Is(plainErr) {
		t.Error("Is() should return false for non-ErrorDefinition targets")
	}

	// errors.Is should also return false
	if errors.Is(roccoErr, plainErr) {
		t.Error("errors.Is should return false for non-ErrorDefinition targets")
	}
}

func TestError_Is_NilTarget(t *testing.T) {
	roccoErr := NewError[NoDetails]("TEST", 400, "test")

	// Is() with nil should return false
	if roccoErr.Is(nil) {
		t.Error("Is() should return false for nil target")
	}
}

func TestSentinelErrors_Exist(t *testing.T) {
	tests := []struct {
		name    string
		err     ErrorDefinition
		code    string
		status  int
		message string
	}{
		{"ErrBadRequest", ErrBadRequest, "BAD_REQUEST", 400, "bad request"},
		{"ErrUnauthorized", ErrUnauthorized, "UNAUTHORIZED", 401, "unauthorized"},
		{"ErrForbidden", ErrForbidden, "FORBIDDEN", 403, "forbidden"},
		{"ErrNotFound", ErrNotFound, "NOT_FOUND", 404, "not found"},
		{"ErrConflict", ErrConflict, "CONFLICT", 409, "conflict"},
		{"ErrPayloadTooLarge", ErrPayloadTooLarge, "PAYLOAD_TOO_LARGE", 413, "payload too large"},
		{"ErrUnprocessableEntity", ErrUnprocessableEntity, "UNPROCESSABLE_ENTITY", 422, "unprocessable entity"},
		{"ErrValidationFailed", ErrValidationFailed, "VALIDATION_FAILED", 422, "validation failed"},
		{"ErrTooManyRequests", ErrTooManyRequests, "TOO_MANY_REQUESTS", 429, "too many requests"},
		{"ErrInternalServer", ErrInternalServer, "INTERNAL_SERVER_ERROR", 500, "internal server error"},
		{"ErrNotImplemented", ErrNotImplemented, "NOT_IMPLEMENTED", 501, "not implemented"},
		{"ErrServiceUnavailable", ErrServiceUnavailable, "SERVICE_UNAVAILABLE", 503, "service unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
			if tt.err.Code() != tt.code {
				t.Errorf("expected code %q, got %q", tt.code, tt.err.Code())
			}
			if tt.err.Status() != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, tt.err.Status())
			}
			if tt.err.Message() != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, tt.err.Message())
			}
		})
	}
}

func TestSentinelErrors_AreUnique(t *testing.T) {
	sentinels := []ErrorDefinition{
		ErrBadRequest,
		ErrUnauthorized,
		ErrForbidden,
		ErrNotFound,
		ErrConflict,
		ErrPayloadTooLarge,
		ErrUnprocessableEntity,
		ErrValidationFailed,
		ErrTooManyRequests,
		ErrInternalServer,
		ErrNotImplemented,
		ErrServiceUnavailable,
	}

	for i, err1 := range sentinels {
		for j, err2 := range sentinels {
			if i != j && err1.Code() == err2.Code() {
				t.Errorf("sentinel errors are not unique: %v == %v", err1.Code(), err2.Code())
			}
		}
	}
}

func TestSentinelErrors_CanBeMatched(t *testing.T) {
	// Create a derived error
	derived := ErrNotFound.WithMessage("user not found")

	// Should match with errors.Is
	if !errors.Is(derived, ErrNotFound) {
		t.Error("derived error should match original sentinel")
	}

	// Should not match different sentinel
	if errors.Is(derived, ErrBadRequest) {
		t.Error("derived error should not match different sentinel")
	}
}

func TestNewError_UserDefined(t *testing.T) {
	type TeapotDetails struct {
		TeaType string `json:"tea_type"`
	}

	// Users should be able to define their own errors with typed details
	ErrTeapot := NewError[TeapotDetails]("TEAPOT", 418, "I'm a teapot")

	if ErrTeapot.Code() != "TEAPOT" {
		t.Errorf("expected code TEAPOT, got %s", ErrTeapot.Code())
	}
	if ErrTeapot.Status() != 418 {
		t.Errorf("expected status 418, got %d", ErrTeapot.Status())
	}

	// Derived errors should match
	derived := ErrTeapot.WithMessage("definitely a teapot")
	if !errors.Is(derived, ErrTeapot) {
		t.Error("derived error should match user-defined sentinel")
	}

	// Details should be properly typed
	withDetails := ErrTeapot.WithDetails(TeapotDetails{TeaType: "Earl Grey"})
	if withDetails.Details().TeaType != "Earl Grey" {
		t.Error("typed details not set correctly")
	}
}

func TestErrorDefinition_DetailsMeta(t *testing.T) {
	// Verify that DetailsMeta returns sentinel metadata
	meta := ErrNotFound.DetailsMeta()
	if meta.TypeName != "NotFoundDetails" {
		t.Errorf("expected TypeName 'NotFoundDetails', got %q", meta.TypeName)
	}
}
