//go:build linux

package execenv

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/cli/help"
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
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := parts[1]

		// Basic key validation to prevent setting weird things?
		// Setenv handles most cases.
		if key == "" {
			continue
		}

		_ = os.Setenv(key, val)
	}
	return scanner.Err()
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "_exec-env",
		Description: "Internal wrapper for DynamicUser environment bridging",
		Usage:       "lynx _exec-env <cmd> [args...]",
	}
}
