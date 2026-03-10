//go:build linux

package execenv

import (
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
		return fmt.Errorf("usage: lynx _exec-env <cmd> [args...]")
	}

	// Load credentials
	credsDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credsDir != "" {
		envPath := credsDir + "/env"
		if err := loadEnv(envPath); err != nil {
			// If we are running under systemd with LoadCredential, this should work.
			// If it fails, log to stderr (which goes to journal) and continue?
			// Or fail fast?
			// User requirement: "Export KEY=VAL lines safely"
			// If we can't read the env, the app might fail.
			fmt.Fprintf(os.Stderr, "lynx: warning: failed to load env from credentials: %v\n", err)
		}
	}

	cmdName := args[0]
	cmdArgs := args

	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return fmt.Errorf("command not found: %s", cmdName)
	}

	// Exec
	env := os.Environ()
	if err := syscall.Exec(cmdPath, cmdArgs, env); err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	// Should not be reached
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
		Usage:       "lynx _exec-env <cmd> [args...]",
	}
}
