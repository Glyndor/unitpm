//go:build linux

// Package execenv implements the internal _exec-env wrapper used by
// --isolation dynamic to bridge LoadCredential env vars into the managed
// process.
package execenv

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/env"
)

// Run executes the _exec-env command.
func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lynxpm _exec-env <cmd> [args...]")
	}

	credsDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credsDir != "" {
		envPath := credsDir + "/env"
		if err := loadEnv(envPath); err != nil {
			// Best-effort: warn to journal and let the child process decide whether to fail.
			fmt.Fprintf(os.Stderr, "lynxpm: warning: failed to load env from credentials: %v\n", err)
		}
	}

	cmdName := args[0]
	cmdArgs := args

	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return fmt.Errorf("command not found: %s", cmdName)
	}

	env := os.Environ()
	if err := syscall.Exec(cmdPath, cmdArgs, env); err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	return nil
}

func loadEnv(path string) error {
	parsed, err := env.ParseFile(path)
	if err != nil {
		return err
	}
	for k, v := range parsed {
		_ = os.Setenv(k, v)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "_exec-env",
		Description: "Internal wrapper for DynamicUser environment bridging",
		Usage:       "lynxpm _exec-env <cmd> [args...]",
		Hidden:      true,
	}
}
