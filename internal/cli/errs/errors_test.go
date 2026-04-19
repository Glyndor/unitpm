package errs

import (
	"errors"
	"testing"
)

func TestIsUsageError(t *testing.T) {
	err := &UsageError{Message: "test"}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Error("Expected errors.As to match UsageError")
	}

	if errors.As(errors.New("test"), &usageErr) {
		t.Error("Expected errors.As to NOT match generic error")
	}
}

func TestUsageError_Error(t *testing.T) {
	err := &UsageError{Message: "test"}
	if err.Error() != "test" {
		t.Errorf("Expected 'test', got '%s'", err.Error())
	}
}
