//go:build linux

package installtools_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/installtools"
)

func TestGetSpec(t *testing.T) {
	spec := installtools.GetSpec()
	if spec.Name != "install-tools" {
		t.Errorf("expected name 'install-tools', got %s", spec.Name)
	}
	if spec.Description == "" {
		t.Error("expected non-empty description")
	}
	// Ensure --system option is documented
	found := false
	for _, opt := range spec.Options {
		if strings.Contains(opt.Long, "--system") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected --system flag in options")
	}
}

func TestRun_Help(t *testing.T) {
	err := installtools.Run([]string{"--help"})
	if err != nil {
		t.Errorf("Run(--help) failed: %v", err)
	}
}

func TestRun_SystemWithoutRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test non-root branch when running as root")
	}
	err := installtools.Run([]string{"--system", "-y"})
	if err == nil {
		t.Fatal("expected error when --system used without root")
	}
	if !strings.Contains(err.Error(), "requires root") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_UserMode(t *testing.T) {
	// User mode (default): no root needed. Point HOME to temp dir.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Auto-yes so it doesn't prompt
	err := installtools.Run([]string{"-y"})
	if err != nil {
		t.Errorf("expected no error in user mode, got %v", err)
	}
	// ~/.local/bin should now exist
	if _, err := os.Stat(home + "/.local/bin"); err != nil {
		t.Errorf("expected ~/.local/bin to be created, got %v", err)
	}
}

func TestRun_UserMode_LongYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := installtools.Run([]string{"--yes"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
