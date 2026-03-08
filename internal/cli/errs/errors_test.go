package errs

import (
	"testing"
)

func TestUsageError(t *testing.T) {
	msg := "invalid argument"
	err := NewUsageError(msg)

	// Check type assertion
	uErr, ok := err.(*UsageError)
	if !ok {
		t.Errorf("NewUsageError() did not return *UsageError, got %T", err)
	}

	// Check message
	if uErr.Message != msg {
		t.Errorf("Expected message %q, got %q", msg, uErr.Message)
	}

	// Check Error() method
	if err.Error() != msg {
		t.Errorf("Error() = %q, want %q", err.Error(), msg)
	}
}
