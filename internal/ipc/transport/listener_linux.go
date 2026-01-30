// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"context"
	"net"
	"os"
	"os/user"
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

	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}

	// Determine if this is the system socket
	// We assume system socket is at /run/lynx/lynx.sock
	isSystem := path == "/run/lynx/lynx.sock"

	if isSystem {
		// System Mode: accessible by root and lynxadm group
		// Try to change group to 'lynxadm'
		g, err := user.LookupGroup("lynxadm")
		if err == nil {
			gid, _ := strconv.Atoi(g.Gid)
			// Change group ownership
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
