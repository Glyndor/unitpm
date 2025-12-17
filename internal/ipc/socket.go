package ipc

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// GetSocketPath returns the OS-specific path for the IPC socket/pipe.
func GetSocketPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	if runtime.GOOS == "windows" {
		// Named pipe: \\.\pipe\lynx-<sid>
		// using SID is safer than username (which can have spaces/backslashes)
		return `\\.\pipe\lynx-` + u.Uid, nil
	}

	// Unix
	baseDir := os.Getenv("XDG_RUNTIME_DIR")
	if baseDir == "" {
		baseDir = os.TempDir()
	}

	// Create a subdirectory for lynx
	sockDir := filepath.Join(baseDir, "lynx-"+u.Uid)

	// Ensure directory exists with 0700
	if err := os.MkdirAll(sockDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Enforce 0700 permissions
	if err := os.Chmod(sockDir, 0700); err != nil {
		return "", fmt.Errorf("failed to set socket directory permissions: %w", err)
	}

	return filepath.Join(sockDir, "lynx.sock"), nil
}
