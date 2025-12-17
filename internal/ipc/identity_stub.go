//go:build !linux && !windows

package ipc

import "net"

func validateIdentity(conn net.Conn) error {
	// TODO: Implement for other platforms (e.g. using getpeereid on macOS)
	// For now rely on file permissions (0700/0600)
	return nil
}
