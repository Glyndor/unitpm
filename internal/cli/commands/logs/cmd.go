package logs

import (
	"bufio"
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

// Run executes the logs command.
func Run(args []string) error {
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
		return fmt.Errorf("missing process ID or name")
	}

	// Resolve target locally
	specs, err := spec.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load specs: %w", err)
	}

	var match *protocol.AppSpec
	for _, s := range specs {
		// Match exact ID, Name, or partial ID
		if s.ID == target || s.Name == target || strings.HasPrefix(s.ID, target) {
			if match != nil && match.ID != s.ID {
				return fmt.Errorf("ambiguous argument '%s': matches multiple processes", target)
			}
			current := s // copy
			match = &current
		}
	}

	if match == nil {
		return fmt.Errorf("process '%s' not found", target)
	}

	stdoutPath, stderrPath, err := paths.ResolveLogPaths(match)
	if err != nil {
		return fmt.Errorf("failed to resolve log paths: %w", err)
	}

	term.Printf("Showing logs for %s (%s)\n", match.Name, match.ID)
	term.Printf("Stdout: %s\n", stdoutPath)
	term.Printf("Stderr: %s\n\n", stderrPath)

	var wg sync.WaitGroup

	if showStdout {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tailFile(stdoutPath, "STDOUT", lines, follow)
		}()
	}

	if showStderr && stderrPath != stdoutPath {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tailFile(stderrPath, "STDERR", lines, follow)
		}()
	}

	wg.Wait()
	return nil
}

func tailFile(path, label string, n int, follow bool) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File might not exist yet if process hasn't started/logged
			if follow {
				fmt.Printf("[%s] File not found, waiting...\n", label)
				// Wait for file creation
				for {
					time.Sleep(1 * time.Second)
					f, err = os.Open(path)
					if err == nil {
						break
					}
				}
			} else {
				fmt.Printf("[%s] File not found\n", label)
				return
			}
		} else {
			fmt.Printf("[%s] Error: %v\n", label, err)
			return
		}
	}
	defer f.Close()

	// Initial Read: Last N lines
	printLastNLines(f, label, n)

	if !follow {
		return
	}

	// Seek to end to ensure we don't re-read what printLastNLines read?
	// printLastNLines should leave offset at end.
	// But printLastNLines might iterate.
	// Simpler: Seek end.
	_, _ = f.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			fmt.Printf("[%s] Error: %v\n", label, err)
			return
		}
		fmt.Printf("[%s] %s", label, line)
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

	_, _ = f.Seek(offset, io.SeekStart)

	scanner := bufio.NewScanner(f)
	// If we seeked, discard first partial line
	if offset > 0 {
		scanner.Scan()
	}

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}

	for _, line := range lines {
		fmt.Printf("[%s] %s\n", label, line)
	}
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "logs",
		Description: "View and follow process logs",
		Usage:       "lynx logs <id|name> [--lines N] [--follow] [--stdout] [--stderr]",
	}
}
