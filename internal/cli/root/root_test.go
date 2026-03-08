package root_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/root"
)

func TestExecute_Help(t *testing.T) {
	// Execute help command should return 0
	code := root.Execute([]string{"help"})
	if code != 0 {
		t.Errorf("Execute(help) = %d, want 0", code)
	}

	code = root.Execute([]string{"--help"})
	if code != 0 {
		t.Errorf("Execute(--help) = %d, want 0", code)
	}

	code = root.Execute([]string{"-h"})
	if code != 0 {
		t.Errorf("Execute(-h) = %d, want 0", code)
	}
}

func TestExecute_Unknown(t *testing.T) {
	code := root.Execute([]string{"unknown-command"})
	if code != 1 {
		t.Errorf("Execute(unknown-command) = %d, want 1", code)
	}
}
