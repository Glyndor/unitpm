//go:build linux

// Package startup implements the startup command.
package startup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Runner interface allows dependency injection for command execution.
type Runner interface {
	Run(name string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// RealRunner implements Runner using exec.CommandContext.
type RealRunner struct{}

// Run executes a command using exec.CommandContext.
func (r *RealRunner) Run(name string, args ...string) (string, string, int, error) {
	// Use context.Background() as per requirement to use exec.CommandContext
	cmd := exec.CommandContext(context.Background(), name, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return stdout, stderr, exitCode, err
}

// Run executes the startup command.
// It accepts a runner to allow testing. If runner is nil, RealRunner is used.
func Run(runner Runner, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	if runner == nil {
		runner = &RealRunner{}
	}

	return runPlatformStartup(runner)
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "startup",
		Usage:       term.BoldString("lynx startup"),
		Description: "Enable and start the Lynx system daemon (lynxd). Supported: Debian/Ubuntu (systemd).",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message for the startup command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
