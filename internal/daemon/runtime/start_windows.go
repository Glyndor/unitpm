//go:build windows
// +build windows

// Package runtime provides process isolation mechanisms.
package runtime

import (
	"errors"
	"os/exec"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// ConfigureProcessIsolation configures the process isolation for the command.
func ConfigureProcessIsolation(_ *exec.Cmd, runAs protocol.RunAsPolicy) error {
	switch runAs.Mode {
	case "self":
		return nil
	case "app_user":
		// Placeholder for Job Objects / Token handling
		return errors.New("ERR_UNSUPPORTED: run_as=app_user not supported in Phase 1")
	case "explicit_user":
		return errors.New("ERR_UNSUPPORTED: run_as=explicit_user not supported in Phase 1")
	default:
		return nil
	}
}
