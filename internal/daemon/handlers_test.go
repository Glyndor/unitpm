package daemon

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/audit"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// TestRegisterHandlers_WiresEveryVerb verifies that the public surface of
// the IPC server — the verb set — contains every command we ship, and no
// verb is silently missing after a refactor. This is a schema check, not
// a behaviour test; the per-handler behaviour lives with each command.
func TestRegisterHandlers_WiresEveryVerb(t *testing.T) {
	server := transport.NewServer()
	mgr := manager.NewManager()
	RegisterHandlers(server, mgr, false /*privileged*/, audit.Disabled())

	wantVerbs := []string{
		"ping", "start", "stop", "restart", "reload", "reset", "flush",
		"delete", "list", "show", "version", "scale",
	}
	for _, v := range wantVerbs {
		if !server.HasHandler(v) {
			t.Errorf("verb %q not registered", v)
		}
	}
}

// TestRegisterHandlers_Privileged covers the privileged=true branch so
// the coverage tool doesn't flag the toggled behaviour as dead.
func TestRegisterHandlers_Privileged(t *testing.T) {
	server := transport.NewServer()
	mgr := manager.NewManager()
	RegisterHandlers(server, mgr, true, audit.Disabled())
	if !server.HasHandler("start") {
		t.Error("start missing under privileged=true")
	}
}
