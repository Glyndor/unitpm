package daemon

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/audit"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// TestRegisterHandlers_WiresEveryVerb catches silent removal of a verb
// after a refactor. Update wantVerbs when adding a new command.
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

func TestRegisterHandlers_Privileged(t *testing.T) {
	server := transport.NewServer()
	mgr := manager.NewManager()
	RegisterHandlers(server, mgr, true, audit.Disabled())
	if !server.HasHandler("start") {
		t.Error("start missing under privileged=true")
	}
}
