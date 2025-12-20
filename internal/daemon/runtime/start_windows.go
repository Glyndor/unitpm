//go:build windows

package runtime

import (
	"fmt"
	"os/exec"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func ConfigureProcessIsolation(cmd *exec.Cmd, runAs protocol.RunAsPolicy) error {
	switch runAs.Mode {
	case "self":
		return nil
	case "app_user":
		// Placeholder for Job Objects / Token handling
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=app_user not supported on Windows yet")
	case "explicit_user":
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=explicit_user not supported on Windows yet")
	default:
		return nil
	}
}
