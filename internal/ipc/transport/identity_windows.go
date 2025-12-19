//go:build windows

package transport

import "net"

func validateIdentity(_ net.Conn) error {
	// Identity validation is handled by the named pipe security descriptor
	return nil
}
