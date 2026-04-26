// Package logs implements the logs command: tails and streams a
// process's stdout/stderr log files merged in chronological order.
package logs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/paths"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

// Sleeper is a function type for pausing execution, usually for polling.
type Sleeper func(time.Duration)

// options bundles parsed flags for the logs command.
type options struct {
	lines      int
	follow     bool
	all        bool
	yes        bool
	noMerge    bool
	since      time.Duration
	grep       string
	target     string
	showStdout bool
	showStderr bool
	explicit   bool
}

// Run executes the logs command.
func Run(args []string) error {
	return runWithContext(context.Background(), args)
}

func runWithContext(ctx context.Context, args []string) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	match, err := resolveTarget(opts.target)
	if err != nil {
		return err
	}

	sources, err := buildSources(match, opts)
	if err != nil {
		return err
	}

	fs, err := buildFilter(opts)
	if err != nil {
		return err
	}

	_, _ = term.Printf("Showing logs for %s (%s)\n", match.Name, match.ID)
	for _, s := range sources {
		_, _ = term.Printf("%s %s\n", colorLabel(s.label), s.path)
	}
	_, _ = term.Printf("\n")

	if opts.noMerge {
		return runLegacySplit(ctx, sources, opts)
	}

	if opts.all {
		if err := guardLargeRead(sources, opts.yes, os.Stdin); err != nil {
			return err
		}
		if err := streamMerge(ctx, os.Stdout, fs, sources...); err != nil {
			return err
		}
	} else {
		if err := boundedTail(os.Stdout, sources, opts.lines, fs); err != nil {
			return err
		}
	}

	if !opts.follow {
		return nil
	}
	return followMerge(ctx, os.Stdout, sources, fs, time.Sleep)
}

func parseArgs(args []string) (options, error) {
	opts := options{lines: 40}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--lines" || arg == "-n" || arg == "--tail":
			if i+1 < len(args) {
				if l, err := strconv.Atoi(args[i+1]); err == nil {
					opts.lines = l
					i++
				}
			}
		case arg == "--follow" || arg == "-f":
			opts.follow = true
		case arg == "--all":
			opts.all = true
		case arg == "--yes" || arg == "-y":
			opts.yes = true
		case arg == "--no-merge":
			opts.noMerge = true
		case arg == "--since":
			if i+1 < len(args) {
				d, err := time.ParseDuration(args[i+1])
				if err != nil {
					return opts, fmt.Errorf("invalid --since duration %q: %w", args[i+1], err)
				}
				opts.since = d
				i++
			}
		case arg == "--grep" || arg == "-g":
			if i+1 < len(args) {
				opts.grep = args[i+1]
				i++
			}
		case arg == "--stdout" || arg == "-o":
			opts.showStdout = true
			opts.explicit = true
		case arg == "--stderr" || arg == "-e":
			opts.showStderr = true
			opts.explicit = true
		case !strings.HasPrefix(arg, "-"):
			opts.target = arg
		}
	}

	if !opts.explicit {
		opts.showStdout = true
		opts.showStderr = true
	}
	if opts.target == "" {
		return opts, errors.New("missing process ID or name")
	}
	return opts, nil
}

func resolveTarget(target string) (*protocol.AppSpec, error) {
	var namespace, nameOrID string
	if idx := strings.Index(target, ":"); idx != -1 {
		namespace = target[:idx]
		nameOrID = target[idx+1:]
	} else {
		nameOrID = target
	}

	specs, err := spec.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load specs: %w", err)
	}

	var match *protocol.AppSpec
	for _, s := range specs {
		ns := s.Namespace
		if ns == "" {
			ns = types.DefaultNamespace
		}
		if namespace != "" && ns != namespace {
			continue
		}
		if s.ID == nameOrID || s.Name == nameOrID || strings.HasPrefix(s.ID, nameOrID) {
			if match != nil && match.ID != s.ID {
				return nil, fmt.Errorf("ambiguous argument '%s': matches multiple processes", target)
			}
			current := s
			match = &current
		}
	}
	if match == nil {
		return nil, fmt.Errorf("process '%s' not found", target)
	}
	return match, nil
}

func buildSources(match *protocol.AppSpec, opts options) ([]streamSource, error) {
	var logsDir, stdout, stderr string
	if match.Logs != nil {
		logsDir = match.Logs.Dir
		stdout = match.Logs.Stdout
		stderr = match.Logs.Stderr
	}
	stdoutPath, stderrPath, err := paths.ResolveLogPaths(match.ID, logsDir, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve log paths: %w", err)
	}

	out := make([]streamSource, 0, 2)
	if opts.showStdout {
		out = append(out, streamSource{path: stdoutPath, label: "STDOUT"})
	}
	// Same path = single physical file. Adding it twice would double
	// every line in the merge.
	if opts.showStderr && stderrPath != stdoutPath {
		out = append(out, streamSource{path: stderrPath, label: "STDERR"})
	}
	return out, nil
}

func buildFilter(opts options) (filter, error) {
	var fs filter
	if opts.since > 0 {
		fs.since = time.Now().Add(-opts.since)
	}
	if opts.grep != "" {
		re, err := regexp.Compile(opts.grep)
		if err != nil {
			return fs, fmt.Errorf("invalid --grep regex: %w", err)
		}
		fs.grep = re
	}
	return fs, nil
}

func colorLabel(label string) string {
	switch label {
	case "STDOUT":
		return term.CyanString("[STDOUT]")
	case "STDERR":
		return term.RedString("[STDERR]")
	default:
		return term.DimString("[%s]", label)
	}
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "logs",
		Aliases:     []string{"log"},
		Description: "View and follow process logs (chronologically merged)",
		Usage:       "lynxpm logs <id|name> [-n N] [--all] [-f] [--since DUR] [--grep RE] [--stdout|--stderr] [--no-merge]",
		Examples: []string{
			`lynxpm logs api`,
			`lynxpm logs api --follow`,
			`lynxpm logs api --tail 100`,
			`lynxpm logs api --all --grep "ERROR"`,
			`lynxpm logs api --since 30m`,
			`lynxpm logs prod:api`,
		},
	}
}
