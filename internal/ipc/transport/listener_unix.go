//go:build !windows

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
	// We assume system socket is at /run/lynxd/lynx.sock
	isSystem := path == "/run/lynxd/lynx.sock"

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
		// Check owner is root or the executing user (e.g. 'lynx')
		stat_t, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, fmt.Errorf("socket directory %s: cannot read ownership (non-Unix filesystem)", dir)
		}
		expectedUID := uint32(os.Getuid())

		if stat_t.Uid != 0 && stat_t.Uid != expectedUID {
			return nil, fmt.Errorf(
				"socket directory %s is owned by uid %d, "+
					"expected root (0) or daemon user (%d)",
				dir, stat_t.Uid, expectedUID)
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
			_ = os.Chown(path, -1, gid)
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
