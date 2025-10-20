package rocco

import (
	"errors"
	"testing"
)

func TestSentinelErrors_Exist(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrBadRequest", ErrBadRequest, "bad request"},
		{"ErrUnauthorized", ErrUnauthorized, "unauthorized"},
		{"ErrForbidden", ErrForbidden, "forbidden"},
		{"ErrNotFound", ErrNotFound, "not found"},
		{"ErrConflict", ErrConflict, "conflict"},
		{"ErrUnprocessableEntity", ErrUnprocessableEntity, "unprocessable entity"},
		{"ErrTooManyRequests", ErrTooManyRequests, "too many requests"},
		{"ErrInternalServer", ErrInternalServer, "internal server error"},
		{"ErrNotImplemented", ErrNotImplemented, "not implemented"},
		{"ErrServiceUnavailable", ErrServiceUnavailable, "service unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
			if tt.err.Error() != tt.msg {
				t.Errorf("expected message %q, got %q", tt.msg, tt.err.Error())
			}
		})
	}
}

func TestSentinelErrors_AreUnique(t *testing.T) {
	sentinels := []error{
		ErrBadRequest,
		ErrUnauthorized,
		ErrForbidden,
		ErrNotFound,
		ErrConflict,
		ErrUnprocessableEntity,
		ErrTooManyRequests,
		ErrInternalServer,
		ErrNotImplemented,
		ErrServiceUnavailable,
	}

	for i, err1 := range sentinels {
		for j, err2 := range sentinels {
			if i != j && errors.Is(err1, err2) {
				t.Errorf("sentinel errors are not unique: %v == %v", err1, err2)
			}
		}
	}
}

func TestSentinelErrors_CanBeWrapped(t *testing.T) {
	wrapped := errors.New("wrapped: " + ErrNotFound.Error())

	// Wrapped errors should not be detected as sentinel errors
	if errors.Is(wrapped, ErrNotFound) {
		t.Error("wrapped error should not match sentinel directly")
	}

	// But wrapping with %w should work
	wrappedCorrectly := errors.Join(ErrNotFound, errors.New("additional context"))
	if !errors.Is(wrappedCorrectly, ErrNotFound) {
		t.Error("properly wrapped error should match sentinel")
	}
}
