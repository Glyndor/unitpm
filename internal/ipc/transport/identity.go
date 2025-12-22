//go:build linux

package transport

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
)

func validateIdentity(conn net.Conn) (*Identity, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("invalid connection type")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var cred *syscall.Ucred
	var credErr error

	err = rawConn.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return nil, err
	}
	if credErr != nil {
		return nil, credErr
	}

	if int(cred.Uid) != os.Getuid() {
		return nil, fmt.Errorf("unauthorized user: %d", cred.Uid)
	}

	return &Identity{
		UID: strconv.Itoa(int(cred.Uid)),
		GID: strconv.Itoa(int(cred.Gid)),
		PID: int(cred.Pid),
	}, nil
}
