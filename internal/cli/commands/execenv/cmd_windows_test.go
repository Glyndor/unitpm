//go:build windows

package execenv_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/execenv"
)

func TestRun_Windows(t *testing.T) {
	err := execenv.Run([]string{})
	if err != nil {
		t.Errorf("Run() failed on Windows: %v", err)
	}
}

func TestGetSpec_Windows(t *testing.T) {
	spec := execenv.GetSpec()
	if spec.Name != "_exec-env" {
		t.Errorf("Expected name '_exec-env', got %s", spec.Name)
	}
}
