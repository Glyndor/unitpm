// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"context"
	"net"
	"time"
)

func dial(path string, timeout time.Duration) (net.Conn, error) {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return d.DialContext(ctx, "unix", path)
}
