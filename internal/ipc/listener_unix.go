//go:build !windows

package ipc

import (
	"net"
	"os"
)

func listen(path string) (net.Listener, error) {
	// Remove existing socket if it exists
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	// Enforce 0600 permissions
	if err := os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}

	return l, nil
}
