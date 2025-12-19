//go:build linux

package transport

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func validateIdentity(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("invalid connection type")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}

	var cred *syscall.Ucred
	var credErr error

	err = rawConn.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}

	if int(cred.Uid) != os.Getuid() {
		return fmt.Errorf("unauthorized user: %d", cred.Uid)
	}

	return nil
}
