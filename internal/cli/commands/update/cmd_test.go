package update_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/update"
)

func TestRun_Validation(t *testing.T) {
	// We can only test flag parsing here without mocking updater calls.
	// But `Run` calls `updater.IsManagedByPackageSystem()` immediately.
	// Since that function uses `os.Executable`, it might be safe-ish, but `dpkg` call will fail on Windows/Mac.
	// However, `IsManagedByPackageSystem` returns false on error, so it should proceed to `updater.Check`.
	// `updater.Check` makes network calls. We should probably avoid calling `Run` in unit tests without mocking.

	// Instead, let's just test GetSpec and PrintHelp which are safe.
	spec := update.GetSpec()
	if spec.Name != "update" {
		t.Errorf("Expected name 'update', got %s", spec.Name)
	}

	// Redirect stdout for PrintHelp? PrintHelp writes to os.Stdout directly.
	// We can't easily capture it without redirecting os.Stdout, which is messy in parallel tests.
	// So we just ensure it doesn't panic.
	update.PrintHelp()
}

func TestRun_InvalidFlags(t *testing.T) {
	var buf bytes.Buffer
	err := update.Run(&buf, []string{"--invalid-flag"})
	if err == nil {
		t.Error("Expected error for invalid flag, got nil")
	}
	if !strings.Contains(err.Error(), "Unknown flag") {
		t.Errorf("Expected 'Unknown flag' error, got %v", err)
	}
}
