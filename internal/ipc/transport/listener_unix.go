//go:build !windows

// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"net"
	"os"
	"syscall"
)

func listen(path string) (net.Listener, error) {
	// Remove existing socket if it exists
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}

	// Set umask to 0077 to ensure the socket is created with 0700 permissions
	// This prevents a race condition where the socket is world-accessible before Chmod
	oldMask := syscall.Umask(0077)
	defer syscall.Umask(oldMask)

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	// Double check permissions (though umask should handle it)
	if err := os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}

	return l, nil
}
