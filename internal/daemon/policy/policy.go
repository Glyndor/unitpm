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
		if !daemonPrivileged {
			return fmt.Errorf("ERR_FORBIDDEN: run_as=app_user requires daemon to be privileged")
		}
		// TODO: Check if the calling user is allowed to run as app_user
		return nil
	case "explicit_user":
		if !daemonPrivileged {
			return fmt.Errorf("ERR_FORBIDDEN: run_as=explicit_user requires daemon to be privileged")
		}
		
		// If on Unix, check if caller is root (UID 0)
		// On Windows, identity.UID is "0" (placeholder), so we skip for now or need better check
		if identity.UID != "0" {
             // If not root, deny switching users
             return fmt.Errorf("ERR_FORBIDDEN: only root can switch users")
		}
		
		return nil
	default:
		return fmt.Errorf("ERR_BAD_REQUEST: invalid run_as mode")
	}
}
