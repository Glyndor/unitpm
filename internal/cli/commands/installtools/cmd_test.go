package installtools_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/installtools"
)

func TestGetSpec(t *testing.T) {
	spec := installtools.GetSpec()
	if spec.Name != "install-tools" {
		t.Errorf("Expected name 'install-tools', got %s", spec.Name)
	}
}

func TestRun_Help(t *testing.T) {
	// Help should return nil (success)
	err := installtools.Run([]string{"--help"})
	if err != nil {
		t.Errorf("Run(--help) failed: %v", err)
	}
}
