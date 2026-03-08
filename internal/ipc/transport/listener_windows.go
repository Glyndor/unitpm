//go:build windows

package transport

import (
	"context"
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

	var lc net.ListenConfig
	return lc.Listen(context.Background(), "unix", path)
}
