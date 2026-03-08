//go:build windows

package runtime

import (
	"os/exec"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// ConfigureProcessIsolation sets up process isolation (namespaces, user, etc.) for the command.
func ConfigureProcessIsolation(cmd *exec.Cmd, runAs protocol.RunAsPolicy) error {
	// Windows doesn't support the same isolation primitives.
	// For now, we just do nothing.
	return nil
}
