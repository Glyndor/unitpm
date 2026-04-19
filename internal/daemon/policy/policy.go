// Package policy implements authorization policies for the daemon.
package policy

import (
	"errors"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// AuthorizeStart checks if the start request is allowed.
func AuthorizeStart(spec protocol.AppSpec, _ *transport.Identity, daemonPrivileged bool) error {
	if spec.Exec.Shell && daemonPrivileged {
		return errors.New("ERR_UNSUPPORTED: shell execution not allowed in system daemon")
	}

	if spec.RunAs == nil {
		return nil
	}

	switch spec.RunAs.Mode {
	case "self":
		return nil
	case "dynamic":
		if !daemonPrivileged {
			return errors.New("ERR_UNSUPPORTED: run_as=dynamic requires system daemon")
		}
		return nil
	case "sandbox":
		// Unprivileged sandbox: user namespaces + landlock + rlimit. Works
		// in both user and system mode without sudo.
		return nil
	case "app_user", "explicit_user":
		// Reserved for future per-app uid/gid isolation. Use "dynamic"
		// (system mode) or "sandbox" (user mode) in the meantime.
		return errors.New(
			"ERR_UNSUPPORTED: run_as=" + spec.RunAs.Mode +
				" is not implemented yet; use 'dynamic' or 'sandbox'")
	default:
		return errors.New("ERR_BAD_REQUEST: invalid run_as mode")
	}
}
