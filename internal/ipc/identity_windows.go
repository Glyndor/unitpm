//go:build windows

package ipc

import "net"

func validateIdentity(conn net.Conn) error {
	// Identity validation is handled by the named pipe security descriptor
	return nil
}
