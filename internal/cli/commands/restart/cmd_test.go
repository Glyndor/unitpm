package restart_test

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/restart"
)

func TestRun_Validation(t *testing.T) {
	err := restart.Run(nil, []string{})
	if err == nil {
		t.Error("Expected error for empty args, got nil")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("Expected 'missing process ID or name', got %v", err)
	}
}
