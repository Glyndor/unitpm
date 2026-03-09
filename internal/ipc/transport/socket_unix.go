//go:build !windows

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
	// 1. Env Override
	if env := os.Getenv("LYNX_SOCKET"); env != "" {
		if !filepath.IsAbs(env) {
			return "", fmt.Errorf("LYNX_SOCKET must be an absolute path, got: %s", env)
		}
		dir := filepath.Dir(env)
		if info, err := os.Stat(dir); err == nil {
			if info.Mode()&0002 != 0 {
				return "", fmt.Errorf("LYNX_SOCKET parent directory %s is world-writable: insecure", dir)
			}
		}
		return env, nil
	}

	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	// 2. System Daemon (root or 'lynx' user)
	// If we are 'lynx' user (system service) or 'root' (admin), default to /run/lynxd/lynx.sock
	if u.Username == "lynx" || u.Uid == "0" {
		return "/run/lynxd/lynx.sock", nil
	}

	// If the user belongs to the 'lynxadm' group, they are administering the system daemon
	gids, err := u.GroupIds()
	if err == nil {
		lynxadmGroup, err := user.LookupGroup("lynxadm")
		if err == nil {
			for _, gid := range gids {
				if gid == lynxadmGroup.Gid {
					return "/run/lynxd/lynx.sock", nil
				}
			}
		}
	}

	// 3. Rootless / User Daemon
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
