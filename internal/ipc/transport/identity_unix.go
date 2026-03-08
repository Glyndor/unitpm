//go:build !windows

package transport

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
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

	// Determine if we are running as the system daemon
	isSystemDaemon := daemonUID == 0
	if !isSystemDaemon {
		if u, err := user.LookupId(strconv.Itoa(daemonUID)); err == nil && u.Username == "lynx" {
			isSystemDaemon = true
		}
	}

	// If we are NOT the system daemon (i.e. we are a rootless user daemon),
	// we firmly require the client UID to match the daemon UID.
	// If we are the system daemon, we rely on the filesystem permissions of the socket
	// (0660 group lynxadm) to protect access.
	if !isSystemDaemon && clientUID != daemonUID {
		return nil, fmt.Errorf("unauthorized user: %d (daemon uid: %d)", clientUID, daemonUID)
	}

	return &Identity{
		UID: strconv.Itoa(int(cred.Uid)),
		GID: strconv.Itoa(int(cred.Gid)),
		PID: int(cred.Pid),
	}, nil
}
