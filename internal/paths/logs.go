package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// GetLogDir resolves the root log directory.
func GetLogDir(configuredDir string) (string, error) {
	if configuredDir != "" {
		if len(configuredDir) > 4096 {
			return "", fmt.Errorf("log dir too long")
		}
		clean := filepath.Clean(configuredDir)
		if strings.Contains(clean, ".."+string(os.PathSeparator)) ||
			strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", fmt.Errorf("invalid log dir")
		}
		return clean, nil
	}
	if os.Geteuid() == 0 {
		return "/var/log/lynx", nil
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome != "" {
		return filepath.Join(stateHome, "lynx/logs"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home: %w", err)
	}
	return filepath.Join(home, ".local/state/lynx/logs"), nil
}

// ResolveLogPaths returns the absolute paths for stdout and stderr logs for a given spec.
func ResolveLogPaths(spec *protocol.AppSpec) (string, string, error) {
	logs := spec.Logs
	if logs == nil {
		logs = &protocol.AppLogs{}
	}

	logDir, err := GetLogDir(logs.Dir)
	if err != nil {
		return "", "", err
	}

	// Per-app log directory
	appLogDir := filepath.Join(logDir, spec.ID)

	// Stdout
	stdoutPath := logs.Stdout
	if stdoutPath == "" {
		stdoutPath = "stdout.log"
	}
	if !filepath.IsAbs(stdoutPath) {
		stdoutPath = filepath.Join(appLogDir, stdoutPath)
	}

	// Stderr
	stderrPath := logs.Stderr
	if stderrPath == "" {
		stderrPath = "stderr.log"
	}
	if !filepath.IsAbs(stderrPath) {
		stderrPath = filepath.Join(appLogDir, stderrPath)
	}

	return stdoutPath, stderrPath, nil
}
