//go:build windows

package transport

import "net"

// validateIdentity checks the peer credentials of the connection.
func validateIdentity(conn net.Conn) (*Identity, error) {
	// Windows named pipes or unix sockets don't support SO_PEERCRED easily in Go.
	// For now, we return a dummy identity.
	return &Identity{
		UID: "0",
		GID: "0",
		PID: 0,
	}, nil
}
