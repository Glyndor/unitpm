package root

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
)

func TestIsHelpRequest_True(t *testing.T) {
	cases := [][]string{
		{"-h"},
		{"--help"},
		{"start", "-h"},
		{"--help", "something"},
		{"foo", "--help", "bar"},
	}
	for _, args := range cases {
		if !isHelpRequest(args) {
			t.Errorf("isHelpRequest(%v) = false, want true", args)
		}
	}
}

func TestIsHelpRequest_False(t *testing.T) {
	cases := [][]string{
		{},
		{"start"},
		{"start", "--name", "api"},
		{"-help"},
		{"help"},
	}
	for _, args := range cases {
		if isHelpRequest(args) {
			t.Errorf("isHelpRequest(%v) = true, want false", args)
		}
	}
}

func TestHandleError_UsageError(t *testing.T) {
	// handleError with a UsageError should not panic and should print to stderr.
	// We just verify it doesn't panic — output goes to os.Stderr.
	err := errs.NewUsageError("missing required flag --name")
	// No panic expected.
	handleError(err, "start")
}

func TestHandleError_GenericError(t *testing.T) {
	// Generic errors print without the usage hint.
	err := &testError{msg: "daemon not running"}
	handleError(err, "list")
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestPrintCommandHelp_UnknownCommand(t *testing.T) {
	// Unknown command name: should return 0 without panicking.
	code := printCommandHelp("unknown-xyz-command")
	if code != 0 {
		t.Errorf("printCommandHelp(unknown) = %d, want 0", code)
	}
}

func TestPrintCommandHelp_KnownCommands(t *testing.T) {
	// Known commands should return 0.
	known := []string{"list", "start", "stop", "restart", "delete", "logs", "version"}
	for _, name := range known {
		code := printCommandHelp(name)
		if code != 0 {
			t.Errorf("printCommandHelp(%q) = %d, want 0", name, code)
		}
	}
}

func TestRunCommand_UnknownReturnsNil(t *testing.T) {
	// Unknown command: should return nil (not an error).
	err := runCommand("nonexistent-command-xyz", nil)
	if err != nil {
		t.Errorf("runCommand(unknown) = %v, want nil", err)
	}
}

// Ensure strings import used.
var _ = strings.Contains
