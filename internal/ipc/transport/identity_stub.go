//go:build !linux && !windows

package transport

import "net"

func validateIdentity(conn net.Conn) (*Identity, error) {
	// TODO: Implement for other platforms (e.g. using getpeereid on macOS)
	// For now rely on file permissions (0700/0600)
	return &Identity{UID: "0", GID: "0", PID: 0}, nil
}
