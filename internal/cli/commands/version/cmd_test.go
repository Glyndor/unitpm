package version_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/version"
)

func TestRun(t *testing.T) {
	// Test basic execution without daemon connection
	var buf bytes.Buffer
	err := version.Run(nil, &buf, []string{})
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Lynx CLI") {
		t.Error("Output missing 'Lynx CLI'")
	}
	// It should print Protocol section even if daemon fails
	if !strings.Contains(output, "Protocol") {
		t.Error("Output missing 'Protocol'")
	}
}

func TestRunHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	// Mock stdout for help printing? 
	// The current implementation prints help to os.Stdout directly in PrintHelp(),
	// so we can't capture it easily without redirecting os.Stdout.
	// But we can check that it returns nil.
	
	err := version.Run(nil, buf, []string{"--help"})
	if err != nil {
		t.Fatalf("Run --help failed: %v", err)
	}
}

func TestRunInvalidFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	err := version.Run(nil, buf, []string{"--invalid"})
	if err == nil {
		t.Fatal("Expected error for invalid flag")
	}

	if !strings.Contains(err.Error(), "Unknown flag") {
		t.Errorf("Expected Unknown flag error, got %v", err)
	}
}

func TestRunUnexpectedArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	err := version.Run(nil, buf, []string{"arg1"})
	if err == nil {
		t.Fatal("Expected error for unexpected args")
	}
	
	if !strings.Contains(err.Error(), "Unexpected arguments") {
		t.Errorf("Expected Unexpected arguments error, got %v", err)
	}
}
