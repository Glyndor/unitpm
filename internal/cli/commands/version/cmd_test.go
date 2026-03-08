//go:build linux

package version

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
)

func TestRun(t *testing.T) {
	// Since Run connects to the daemon, we can only easily test the offline part (CLI version)
	// or mock the transport (which is harder here as it uses transport.NewClient internally).
	// However, we can test flag parsing and basic output structure when daemon is offline.
	
	// Note: Run() returns nil when daemon is offline, just prints what it can.

	buf := new(bytes.Buffer)
	err := Run(buf, []string{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Lynx CLI") {
		t.Error("Output should contain 'Lynx CLI'")
	}
	if !strings.Contains(output, "Version") {
		t.Error("Output should contain 'Version'")
	}
}

func TestRunHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	// Mock stdout for help printing? 
	// The current implementation prints help to os.Stdout directly in PrintHelp(),
	// so we can't capture it easily without redirecting os.Stdout.
	// But we can check that it returns nil.
	
	err := Run(buf, []string{"--help"})
	if err != nil {
		t.Fatalf("Run --help failed: %v", err)
	}
}

func TestRunInvalidFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	err := Run(buf, []string{"--invalid"})
	if err == nil {
		t.Fatal("Expected error for invalid flag")
	}

	var usageErr *errs.UsageError
	if !strings.Contains(err.Error(), "Unknown flag") {
		t.Errorf("Expected Unknown flag error, got %v", err)
	}
}

func TestRunUnexpectedArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	err := Run(buf, []string{"arg1"})
	if err == nil {
		t.Fatal("Expected error for unexpected args")
	}
	
	if !strings.Contains(err.Error(), "Unexpected arguments") {
		t.Errorf("Expected Unexpected arguments error, got %v", err)
	}
}
