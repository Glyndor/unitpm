package apply_test

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/apply"
)

func TestRun_Validation(t *testing.T) {
	err := apply.Run(nil, []string{})
	if err == nil {
		t.Error("Expected error for empty args, got nil")
	}
	if !strings.Contains(err.Error(), "missing Lynxfile path") {
		t.Errorf("Expected 'missing Lynxfile path', got %v", err)
	}
}
