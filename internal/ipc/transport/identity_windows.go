//go:build windows

package transport

import "net"

func validateIdentity(_ net.Conn) (*Identity, error) {
	// Identity validation is handled by the named pipe security descriptor
	return &Identity{UID: "0", GID: "0", PID: 0}, nil
}
