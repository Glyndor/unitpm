// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

func listen(path string) (net.Listener, error) {
	// Remove existing socket if it exists
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}

	// Set umask to 0077 to ensure the socket is created with 0700 permissions by default
	// We will relax this for system socket later
	oldMask := syscall.Umask(0077)
	defer syscall.Umask(oldMask)

	// Determine if this is the system socket
	// We assume system socket is at /run/lynx/lynx.sock
	isSystem := path == "/run/lynx/lynx.sock"

	if isSystem {
		dir := filepath.Dir(path)
		// Ensure directory exists with safe permissions (0755 root:root)
		// We don't change ownership here (assuming started as root), but we enforce mode.
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create socket directory: %w", err)
		}

		// Security Check: Directory must not be world-writable
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to stat socket directory: %w", err)
		}
		mode := info.Mode()
		if mode&0002 != 0 {
			return nil, fmt.Errorf("socket directory %s is world-writable (mode %o): insecure", dir, mode)
		}
		// Check owner is root
		stat_t := info.Sys().(*syscall.Stat_t)
		if stat_t.Uid != 0 {
			return nil, fmt.Errorf("socket directory %s is not owned by root (uid: %d)", dir, stat_t.Uid)
		}
	}

	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}

	if isSystem {
		// System Mode: accessible by root and lynxadm group
		// Try to change group to 'lynxadm'
		g, err := user.LookupGroup("lynxadm")
		if err == nil {
			gid, _ := strconv.Atoi(g.Gid)
			// Change group ownership of the SOCKET
			if err := os.Chown(path, -1, gid); err != nil {
				// Log error? We don't have logger here.
				// But failing to set group might be okay if we are not running as a user who can do it (e.g. dev)
				// However, in production it should work.
			}
		}

		// Set permissions to 0660 (rw-rw----)
		if err := os.Chmod(path, 0660); err != nil {
			_ = l.Close()
			return nil, err
		}
	} else {
		// User Mode: 0600 (rw-------)
		if err := os.Chmod(path, 0600); err != nil {
			_ = l.Close()
			return nil, err
		}
	}

	return l, nil
}
