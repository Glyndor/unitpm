//go:build windows

// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func dial(path string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(path, &timeout)
}
