//go:build windows

package ipc

import (
	"net"

	"github.com/Microsoft/go-winio"
)

func listen(path string) (net.Listener, error) {
	// SDDL string to restrict access:
	// D: Discretionary ACL
	// P: Protected (no inheritance)
	// (A;;GA;;;SY): Allow Generic All to Local System
	// (A;;GA;;;BA): Allow Generic All to Built-in Administrators
	// (A;;GA;;;OW): Allow Generic All to Owner
	config := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;OW)",
	}

	return winio.ListenPipe(path, config)
}
