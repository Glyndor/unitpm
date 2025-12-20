//go:build !windows

package runtime

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func ConfigureProcessIsolation(cmd *exec.Cmd, runAs protocol.RunAsPolicy) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	switch runAs.Mode {
	case "self":
		return nil
	case "app_user":
		// Run as "nobody"
		u, err := user.Lookup("nobody")
		if err != nil {
			return fmt.Errorf("failed to lookup user 'nobody': %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
		return nil
	case "explicit_user":
		if runAs.Username == "" {
			return fmt.Errorf("username required for explicit_user")
		}
		u, err := user.Lookup(runAs.Username)
		if err != nil {
			return fmt.Errorf("failed to lookup user '%s': %w", runAs.Username, err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)

		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
		return nil
	default:
		return nil
	}
}
