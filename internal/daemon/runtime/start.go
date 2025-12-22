//go:build linux

// Package runtime provides process runtime configuration.
package runtime

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// ConfigureProcessIsolation sets up process isolation (namespaces, user, etc.) for the command.
func ConfigureProcessIsolation(cmd *exec.Cmd, runAs protocol.RunAsPolicy) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	switch runAs.Mode {
	case "self":
		return nil
	case "app_user":
		// TODO: Phase 2
		return errors.New("ERR_UNSUPPORTED: run_as=app_user not supported in Phase 1")
	case "explicit_user":
		// TODO: Phase 2
		return errors.New("ERR_UNSUPPORTED: run_as=explicit_user not supported in Phase 1")
	default:
		return nil
	}
}
