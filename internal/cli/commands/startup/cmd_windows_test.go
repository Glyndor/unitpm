//go:build windows

package startup_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/startup"
)

func TestRun_Windows(t *testing.T) {
	err := startup.Run(nil, []string{})
	if err != nil {
		t.Errorf("Run() failed on Windows: %v", err)
	}
}

func TestGetSpec_Windows(t *testing.T) {
	spec := startup.GetSpec()
	if spec.Name != "startup" {
		t.Errorf("Expected name 'startup', got %s", spec.Name)
	}
}
