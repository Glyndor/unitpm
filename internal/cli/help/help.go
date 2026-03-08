// Package help provides a centralized help renderer for the CLI.
package help

import (
	"fmt"
	"io"
	"strings"

	"github.com/Jaro-c/Lynx/internal/term"
)

// Option represents a command-line flag/option.
type Option struct {
	Short       string
	Long        string
	Description string
}

// CommandSpec defines the metadata for a CLI command.
type CommandSpec struct {
	Name        string
	Aliases     []string
	Usage       string
	Description string
	Options     []Option
}

// RenderCommandHelp prints the help output for a single command.
func RenderCommandHelp(w io.Writer, spec CommandSpec) {
	// Build effective options
	options := make([]Option, 0, len(spec.Options)+1)
	options = append(options, spec.Options...)

	hasHelp := false
	for _, opt := range options {
		if opt.Short == "-h" || opt.Long == "--help" {
			hasHelp = true
			break
		}
	}

	if !hasHelp {
		options = append(options, Option{
			Short:       "-h",
			Long:        "--help",
			Description: "Show this help message.",
		})
	}

	// Update usage if needed
	usage := spec.Usage
	if len(options) > 0 &&
		!strings.Contains(usage, "[options]") &&
		!strings.Contains(usage, "[flags]") {
		usage += " [options]"
	}

	// 1. Usage
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Usage:"))
	_, _ = fmt.Fprintf(w, "  %s\n", usage)

	// 2. Description
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Description:"))
	lines := strings.Split(spec.Description, "\n")
	for _, line := range lines {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Options:"))

	// Calculate padding
	maxLen := 0
	for _, opt := range options {
		// Length of "-s, --long"
		l := len(opt.Short) + 2 + len(opt.Long) // 2 for ", "
		if l > maxLen {
			maxLen = l
		}
	}

	for _, opt := range options {
		flags := fmt.Sprintf("%s, %s", opt.Short, opt.Long)
		padding := strings.Repeat(" ", maxLen-len(flags)+4)
		_, _ = fmt.Fprintf(w, "  %s%s%s\n", term.BoldString("%s", flags), padding, opt.Description)
	}
	_, _ = fmt.Fprintln(w)
}

// RenderRootHelp prints the help output for the root command.
func RenderRootHelp(w io.Writer, specs []CommandSpec, showCommands bool) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Usage:"))
	_, _ = fmt.Fprintf(w, "  lynx <command> [flags]\n")

	if showCommands {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Commands:"))

		// Calculate padding
		maxLen := 0
		displayNames := make([]string, len(specs))
		for i, spec := range specs {
			name := spec.Name
			if len(spec.Aliases) > 0 {
				name = fmt.Sprintf("%s, %s", spec.Name, strings.Join(spec.Aliases, ", "))
			}
			displayNames[i] = name
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}

		for i, spec := range specs {
			padding := strings.Repeat(" ", maxLen-len(displayNames[i])+3)
			_, _ = fmt.Fprintf(
				w,
				"  %s%s%s\n",
				term.BoldString("%s", displayNames[i]),
				padding,
				spec.Description,
			)
		}
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Get Help:"))
	_, _ = fmt.Fprintf(w, "  lynx --help\n")
	_, _ = fmt.Fprintf(w, "  lynx <command> --help\n")
}

// IsHelp checks if the arguments contain a help flag (-h, --help, or -help).
func IsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			return true
		}
	}
	return false
}
