// Package daemon provides the core daemon logic and initialization.
package daemon

import (
	"context"

	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/version"
)

// RegisterHandlers registers all daemon IPC handlers.
func RegisterHandlers(server *transport.Server, mgr *manager.Manager, privileged bool) {
	// Register ping handler
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
	})

	// Register start handler
	server.Register("start", handlers.StartHandler(mgr, privileged))

	// Register stop handler
	server.Register("stop", func(
		_ context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID string `json:"id"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		if err := mgr.Stop(args.ID); err != nil {
			return nil, err
		}

		return jsonx.Marshal(map[string]string{"status": "stopped"})
	})

	// Register list handler (replacing status)
	// Returns a list of processes with their detailed status
	server.Register("list", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(mgr.List())
	})

	// Register version handler
	server.Register("version", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(version.Get())
	})
}
