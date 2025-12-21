package policy

import (
	"fmt"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// AuthorizeStart checks if the start request is allowed.
func AuthorizeStart(spec protocol.StartSpec, identity *transport.Identity, daemonPrivileged bool) error {
	switch spec.RunAs.Mode {
	case "self":
		// User can always run as themselves
		return nil
	case "app_user":
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=app_user not supported in Phase 1")
	case "explicit_user":
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=explicit_user not supported in Phase 1")
	default:
		return fmt.Errorf("ERR_BAD_REQUEST: invalid run_as mode")
	}
}
