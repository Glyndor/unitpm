package help_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/help"
)

func TestIsHelp(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{}, false},
		{[]string{"start"}, false},
		{[]string{"-h"}, true},
		{[]string{"--help"}, true},
		{[]string{"-help"}, true},
		{[]string{"start", "-h"}, true},
		{[]string{"--name", "foo", "--help"}, true},
	}

	for _, tt := range tests {
		if got := help.IsHelp(tt.args); got != tt.want {
			t.Errorf("IsHelp(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestRenderCommandHelp(t *testing.T) {
	spec := help.CommandSpec{
		Name:        "test-cmd",
		Usage:       "lynx test-cmd [flags]",
		Description: "A test command description.",
		Options: []help.Option{
			{Short: "-f", Long: "--flag", Description: "A flag description"},
		},
	}

	var buf bytes.Buffer
	help.RenderCommandHelp(&buf, spec)

	output := buf.String()

	// Check for key sections
	if !strings.Contains(output, "Usage:") {
		t.Error("Output missing 'Usage:' section")
	}
	if !strings.Contains(output, "lynx test-cmd [flags]") {
		t.Error("Output missing usage string")
	}
	if !strings.Contains(output, "Description:") {
		t.Error("Output missing 'Description:' section")
	}
	if !strings.Contains(output, "A test command description.") {
		t.Error("Output missing description text")
	}
	if !strings.Contains(output, "Options:") {
		t.Error("Output missing 'Options:' section")
	}
	if !strings.Contains(output, "-f, --flag") {
		t.Error("Output missing flag definition")
	}
	if !strings.Contains(output, "A flag description") {
		t.Error("Output missing flag description")
	}
}
