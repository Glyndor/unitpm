// Package policy implements authorization policies for the daemon.
package policy

import (
	"errors"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// AuthorizeStart checks if the start request is allowed.
func AuthorizeStart(spec protocol.AppSpec, _ *transport.Identity, _ bool) error {
	if spec.RunAs == nil {
		// Default to self if not specified, which is allowed
		return nil
	}

	switch spec.RunAs.Mode {
	case "self":
		// User can always run as themselves
		return nil
	case "app_user":
		return errors.New("ERR_UNSUPPORTED: run_as=app_user not supported in Phase 1")
	case "explicit_user":
		return errors.New("ERR_UNSUPPORTED: run_as=explicit_user not supported in Phase 1")
	default:
		return errors.New("ERR_BAD_REQUEST: invalid run_as mode")
	}
}
