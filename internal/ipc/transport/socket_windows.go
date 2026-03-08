//go:build windows

package transport

import (
	"os"
	"path/filepath"
)

// GetSocketPath returns the OS-specific path for the IPC socket/pipe.
func GetSocketPath() (string, error) {
	if env := os.Getenv("LYNX_SOCKET"); env != "" {
		return env, nil
	}
	return filepath.Join(os.TempDir(), "lynx.sock"), nil
}
