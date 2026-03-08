package monit_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/monit"
)

func TestGetSpec(t *testing.T) {
	spec := monit.GetSpec()
	if spec.Name != "monit" {
		t.Errorf("Expected name 'monit', got %s", spec.Name)
	}
}
