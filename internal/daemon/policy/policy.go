package policy

import (
	"fmt"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// AuthorizeStart checks if the start request is allowed.
func AuthorizeStart(spec protocol.StartSpec, privileged bool) error {
	switch spec.RunAs.Mode {
	case "self":
		return nil
	case "app_user":
		if !privileged {
			return fmt.Errorf("ERR_FORBIDDEN: run_as=app_user requires privileged mode")
		}
		// TODO: Implement actual app_user switching logic check if needed
		return nil
	case "explicit_user":
		return fmt.Errorf("ERR_UNSUPPORTED: run_as=explicit_user is not supported yet")
	default:
		return fmt.Errorf("ERR_BAD_REQUEST: invalid run_as mode")
	}
}
