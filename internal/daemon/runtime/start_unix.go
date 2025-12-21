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
		// TODO: Phase 2
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=app_user not supported in Phase 1")
	case "explicit_user":
		// TODO: Phase 2
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=explicit_user not supported in Phase 1")
	default:
		return nil
	}
}
