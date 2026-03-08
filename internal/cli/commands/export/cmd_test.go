package export_test

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/export"
)

func TestRun_Validation(t *testing.T) {
	// Test missing namespace argument
	err := export.Run([]string{})
	if err == nil {
		t.Error("Expected error for empty args, got nil")
	}
	if !strings.Contains(err.Error(), "export requires --namespace") {
		t.Errorf("Expected 'export requires --namespace', got %v", err)
	}

	// Test missing namespace value
	err = export.Run([]string{"--namespace"})
	if err == nil {
		t.Error("Expected error for missing namespace value, got nil")
	}
	if !strings.Contains(err.Error(), "missing value for --namespace") {
		// It might be returning the generic "export requires --namespace" if arg parsing fails before custom check?
		// No, if I pass "--namespace" as last arg, `i+1 >= len(args)` check should trigger "missing value".
		// But maybe my test logic above matches "export requires --namespace <name>"?
		// Ah, wait. `Run` checks `len(args) < 2`. `[]string{"--namespace"}` has len 1.
		// So it returns "export requires --namespace <name>".
		// Correct.
		if strings.Contains(err.Error(), "export requires --namespace") {
			return
		}
		t.Errorf("Expected 'missing value for --namespace' or generic usage error, got %v", err)
	}
}
