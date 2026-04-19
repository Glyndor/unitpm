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
	// Examples are shown at the bottom of `lynx <cmd> --help`. Each string
	// is printed verbatim, indented.
	Examples []string
	// Hidden excludes the command from `lynx` / `lynxpm help` output while
	// keeping it invokable. Use for internal wrappers.
	Hidden bool
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

	// Pre-compute the flag label for each option, handling short-only,
	// long-only, and both cases without leaving a stray leading comma.
	labels := make([]string, len(options))
	maxLen := 0
	for i, opt := range options {
		labels[i] = flagLabel(opt)
		if n := len(labels[i]); n > maxLen {
			maxLen = n
		}
	}

	for i, opt := range options {
		padding := strings.Repeat(" ", maxLen-len(labels[i])+4)
		_, _ = fmt.Fprintf(w, "  %s%s%s\n", term.BoldString("%s", labels[i]), padding, opt.Description)
	}
	_, _ = fmt.Fprintln(w)

	// 4. Examples
	if len(spec.Examples) > 0 {
		_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Examples:"))
		for _, ex := range spec.Examples {
			_, _ = fmt.Fprintf(w, "  %s\n", ex)
		}
		_, _ = fmt.Fprintln(w)
	}
}

// flagLabel formats an option's flag names for help output, leaving out
// the comma when only one form is present.
func flagLabel(opt Option) string {
	switch {
	case opt.Short != "" && opt.Long != "":
		return fmt.Sprintf("%s, %s", opt.Short, opt.Long)
	case opt.Short != "":
		return opt.Short
	case opt.Long != "":
		return "    " + opt.Long
	default:
		return ""
	}
}

// RenderRootHelp prints the help output for the root command.
func RenderRootHelp(w io.Writer, specs []CommandSpec, showCommands bool) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Usage:"))
	_, _ = fmt.Fprintf(w, "  lynx <command> [flags]\n")

	if showCommands {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Commands:"))

		// Filter out hidden internal commands (e.g. _exec-env, _exec-sandbox).
		visible := make([]CommandSpec, 0, len(specs))
		for _, s := range specs {
			if s.Hidden {
				continue
			}
			visible = append(visible, s)
		}

		maxLen := 0
		displayNames := make([]string, len(visible))
		for i, spec := range visible {
			name := spec.Name
			if len(spec.Aliases) > 0 {
				name = fmt.Sprintf("%s, %s", spec.Name, strings.Join(spec.Aliases, ", "))
			}
			displayNames[i] = name
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}

		for i, spec := range visible {
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
