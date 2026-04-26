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

func TestRenderCommandHelp_AppendsHelpFlag(t *testing.T) {
	spec := help.CommandSpec{Name: "x", Usage: "lynx x", Description: "d"}
	var buf bytes.Buffer
	help.RenderCommandHelp(&buf, spec)
	out := buf.String()
	if !strings.Contains(out, "-h, --help") {
		t.Error("expected auto-appended -h/--help")
	}
	if !strings.Contains(out, "[options]") {
		t.Error("expected usage augmented with [options]")
	}
}

func TestRenderCommandHelp_KeepsExistingHelp(t *testing.T) {
	spec := help.CommandSpec{
		Name: "x", Usage: "lynx x [flags]", Description: "d",
		Options: []help.Option{{Short: "-h", Long: "--help", Description: "custom"}},
	}
	var buf bytes.Buffer
	help.RenderCommandHelp(&buf, spec)
	out := buf.String()
	if strings.Count(out, "--help") != 1 {
		t.Errorf("expected one --help, got %d", strings.Count(out, "--help"))
	}
	if !strings.Contains(out, "custom") {
		t.Error("expected custom description preserved")
	}
	if strings.Contains(out, "[options]") {
		t.Error("usage already had [flags], should not append [options]")
	}
}

func TestRenderCommandHelp_LongOnlyShortOnly(t *testing.T) {
	spec := help.CommandSpec{
		Name: "x", Usage: "lynx x", Description: "d",
		Options: []help.Option{
			{Short: "-v", Description: "short only"},
			{Long: "--verbose", Description: "long only"},
		},
	}
	var buf bytes.Buffer
	help.RenderCommandHelp(&buf, spec)
	out := buf.String()
	if strings.Contains(out, ", --verbose") {
		t.Error("long-only option should not have leading comma")
	}
	if !strings.Contains(out, "    --verbose") {
		t.Error("long-only option should be indented to align with short forms")
	}
}

func TestRenderCommandHelp_WithExamples(t *testing.T) {
	spec := help.CommandSpec{
		Name: "x", Usage: "lynx x", Description: "d",
		Examples: []string{"lynx x foo", "lynx x bar"},
	}
	var buf bytes.Buffer
	help.RenderCommandHelp(&buf, spec)
	out := buf.String()
	if !strings.Contains(out, "Examples:") {
		t.Error("expected Examples section")
	}
	if !strings.Contains(out, "lynx x foo") || !strings.Contains(out, "lynx x bar") {
		t.Error("expected example lines rendered")
	}
}

func TestRenderRootHelp_HidesHidden(t *testing.T) {
	specs := []help.CommandSpec{
		{Name: "start", Description: "Start app"},
		{Name: "stop", Aliases: []string{"halt"}, Description: "Stop app"},
		{Name: "_hidden", Description: "Internal", Hidden: true},
	}
	var buf bytes.Buffer
	help.RenderRootHelp(&buf, specs, true)
	out := buf.String()
	for _, want := range []string{"Usage:", "Commands:", "start", "Start app", "stop, halt", "Get Help:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output", want)
		}
	}
	if strings.Contains(out, "_hidden") {
		t.Error("hidden command leaked into root help")
	}
}

func TestRenderRootHelp_NoCommandsSection(t *testing.T) {
	var buf bytes.Buffer
	help.RenderRootHelp(&buf, nil, false)
	out := buf.String()
	if strings.Contains(out, "Commands:") {
		t.Error("Commands section should be hidden when showCommands=false")
	}
	if !strings.Contains(out, "Get Help:") {
		t.Error("expected Get Help section")
	}
}
