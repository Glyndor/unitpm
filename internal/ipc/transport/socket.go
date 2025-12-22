//go:build linux

// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// GetSocketPath returns the OS-specific path for the IPC socket/pipe.
func GetSocketPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
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
