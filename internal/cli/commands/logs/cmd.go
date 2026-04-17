// Package logs implements the logs command: tails and streams a
// process's stdout/stderr log files.
//
//nolint:gocognit,cyclop,nestif,gocyclo
package logs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/paths"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Sleeper is a function type for pausing execution, usually for polling.
type Sleeper func(time.Duration)

// Run executes the logs command.
func Run(args []string) error {
	return runWithContext(context.Background(), args)
}

func runWithContext(ctx context.Context, args []string) error {
	var (
		lines      = 200
		follow     = false
		showStdout = false
		showStderr = false
		target     string
		explicit   = false
	)

	// Simple flag parsing
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--lines" || arg == "-n":
			if i+1 < len(args) {
				if l, err := strconv.Atoi(args[i+1]); err == nil {
					lines = l
					i++
				}
			}
		case arg == "--follow" || arg == "-f":
			follow = true
		case arg == "--stdout" || arg == "-o":
			showStdout = true
			explicit = true
		case arg == "--stderr" || arg == "-e":
			showStderr = true
			explicit = true
		case !strings.HasPrefix(arg, "-"):
			target = arg
		}
	}

	if !explicit {
		showStdout = true
		showStderr = true
	}

	if target == "" {
		return errors.New("missing process ID or name")
	}

	var namespace string
	var nameOrID string
	if idx := strings.Index(target, ":"); idx != -1 {
		namespace = target[:idx]
		nameOrID = target[idx+1:]
	} else {
		nameOrID = target
	}

	specs, err := spec.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load specs: %w", err)
	}

	var match *protocol.AppSpec
	for _, s := range specs {
		ns := s.Namespace
		if ns == "" {
			ns = "default"
		}
		if namespace != "" && ns != namespace {
			continue
		}
		if s.ID == nameOrID || s.Name == nameOrID || strings.HasPrefix(s.ID, nameOrID) {
			if match != nil && match.ID != s.ID {
				return fmt.Errorf("ambiguous argument '%s': matches multiple processes", target)
			}
			current := s
			match = &current
		}
	}

	if match == nil {
		return fmt.Errorf("process '%s' not found", target)
	}

	var logsDir, stdout, stderr string
	if match.Logs != nil {
		logsDir = match.Logs.Dir
		stdout = match.Logs.Stdout
		stderr = match.Logs.Stderr
	}

	stdoutPath, stderrPath, err := paths.ResolveLogPaths(match.ID, logsDir, stdout, stderr)
	if err != nil {
		return fmt.Errorf("failed to resolve log paths: %w", err)
	}

	_, _ = term.Printf("Showing logs for %s (%s)\n", match.Name, match.ID)
	_, _ = term.Printf("Stdout: %s\n", stdoutPath)
	_, _ = term.Printf("Stderr: %s\n\n", stderrPath)

	var wg sync.WaitGroup

	if showStdout {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tailFile(ctx, stdoutPath, "STDOUT", lines, follow, time.Sleep)
		}()
	}

	if showStderr && stderrPath != stdoutPath {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tailFile(ctx, stderrPath, "STDERR", lines, follow, time.Sleep)
		}()
	}

	wg.Wait()
	return nil
}

func tailFile(ctx context.Context, path, label string, n int, follow bool, sleep Sleeper) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File might not exist yet if process hasn't started/logged
			if follow {
				fmt.Printf("%s File not found, waiting...\n", colorLabel(label))
				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					sleep(1 * time.Second)
					f, err = os.Open(path)
					if err == nil {
						break
					}
					if !os.IsNotExist(err) {
						fmt.Printf("%s %s\n", colorLabel(label), term.RedString("Error: %v", err))
						return
					}
				}
			} else {
				fmt.Printf("%s File not found\n", colorLabel(label))
				return
			}
		} else {
			fmt.Printf("%s %s\n", colorLabel(label), term.RedString("Error: %v", err))
			return
		}
	}
	defer func() { _ = f.Close() }()

	// Initial Read: Last N lines
	printLastNLines(f, label, n)

	if !follow {
		return
	}

	_, _ = f.Seek(0, io.SeekEnd) //nolint:errcheck

	reader := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				sleep(200 * time.Millisecond)
				continue
			}
			fmt.Printf("%s %s\n", colorLabel(label), term.RedString("Error: %v", err))
			return
		}
		fmt.Printf("%s %s", colorLabel(label), line)
	}
}

func printLastNLines(f *os.File, label string, n int) {
	// Simple implementation: Read full file if small, else seek
	stat, err := f.Stat()
	if err != nil {
		return
	}

	fileSize := stat.Size()
	// Heuristic: average line 100 bytes
	offset := fileSize - int64(n*150)
	if offset < 0 {
		offset = 0
	}

	_, _ = f.Seek(offset, io.SeekStart) //nolint:errcheck

	scanner := bufio.NewScanner(f)
	// If we seeked, discard first partial line
	if offset > 0 {
		scanner.Scan()
	}

	ring := make([]string, n)
	idx, count := 0, 0
	for scanner.Scan() {
		ring[idx%n] = scanner.Text()
		idx++
		count++
	}

	total := count
	if total > n {
		total = n
	}
	start := idx % n
	if count < n {
		start = 0
	}
	for i := 0; i < total; i++ {
		fmt.Printf("%s %s\n", colorLabel(label), ring[(start+i)%n])
	}
}

func colorLabel(label string) string {
	switch label {
	case "STDOUT":
		return term.CyanString("[STDOUT]")
	case "STDERR":
		return term.YellowString("[STDERR]")
	default:
		return term.DimString("[%s]", label)
	}
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "logs",
		Aliases:     []string{"log"},
		Description: "View and follow process logs",
		Usage:       "lynx logs <id|name> [--lines N] [--follow] [--stdout] [--stderr]",
		Examples: []string{
			`lynx logs api`,
			`lynx logs api --follow`,
			`lynx logs api --lines 100 --stderr`,
			`lynx logs prod:api`,
		},
	}
}
