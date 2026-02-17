//go:build linux

package transport

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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

	daemonUID := os.Getuid()
	clientUID := int(cred.Uid)

	if allowStr := os.Getenv("LYNX_IPC_ALLOW_UIDS"); allowStr != "" {
		allowed := false
		for _, raw := range strings.Split(allowStr, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			id, err := strconv.Atoi(raw)
			if err != nil {
				continue
			}
			if id == clientUID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("unauthorized user %d: not in allowlist", clientUID)
		}
	}

	// If daemon is root (0), we rely on socket permissions (0660 group lynxadm).
	// If daemon is user, we require exact UID match.
	if daemonUID != 0 && clientUID != daemonUID {
		return nil, fmt.Errorf("unauthorized user: %d (daemon uid: %d)", clientUID, daemonUID)
	}

	return &Identity{
		UID: strconv.Itoa(int(cred.Uid)),
		GID: strconv.Itoa(int(cred.Gid)),
		PID: int(cred.Pid),
	}, nil
}
