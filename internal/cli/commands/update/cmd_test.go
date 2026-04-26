package update_test

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/update"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/updater"
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

func TestRun_Help(t *testing.T) {
	var buf bytes.Buffer
	err := update.Run(&buf, []string{"--help"})
	if err != nil {
		t.Errorf("expected no error for --help, got %v", err)
	}
}

func TestRun_UnexpectedArgs(t *testing.T) {
	var buf bytes.Buffer
	err := update.Run(&buf, []string{"extra-positional-arg"})
	if err == nil {
		t.Fatal("expected error for unexpected positional args")
	}
	if !strings.Contains(err.Error(), "Unexpected arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_ManagedApplyWithoutForce(t *testing.T) {
	if !updater.IsManagedByPackageSystem() {
		t.Skip("test binary is not package-managed")
	}
	var buf bytes.Buffer
	err := update.Run(&buf, []string{"--apply"})
	if err == nil || !strings.Contains(err.Error(), "system package manager") {
		t.Errorf("expected managed-package guard, got %v", err)
	}
}

func TestRun_QuietSilences(t *testing.T) {
	prev := term.IsQuiet()
	term.SetQuiet(true)
	t.Cleanup(func() { term.SetQuiet(prev) })

	var buf bytes.Buffer
	// Network call to updater.Check may fail without internet; that's fine — we only
	// care that no progress text was written to buf when quiet mode is active.
	_ = update.Run(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("quiet mode should silence stdout, got: %q", buf.String())
	}
	_ = runtime.GOARCH // keep import live for other tests
}
