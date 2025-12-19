//go:build !windows

// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"net"
	"time"
)

func dial(path string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", path, timeout)
}
