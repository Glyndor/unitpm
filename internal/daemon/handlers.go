//go:build linux

// Package daemon provides the core daemon logic and initialization.
package daemon

import (
	"context"
	"encoding/json"

	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/version"
)

// RegisterHandlers registers all daemon IPC handlers.
func RegisterHandlers(server *transport.Server, mgr *manager.Manager, privileged bool) {
	// Register ping handler
	server.Register("ping", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	// Register start handler
	server.Register("start", handlers.StartHandler(mgr, privileged))

	// Register stop handler
	server.Register("stop", func(
		_ context.Context,
		params json.RawMessage,
	) (json.RawMessage, error) {
		var args struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		if err := mgr.Stop(args.ID); err != nil {
			return nil, err
		}

		return json.Marshal(map[string]string{"status": "stopped"})
	})

	// Register list handler (replacing status)
	// Returns a list of processes with their detailed status
	server.Register("list", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(mgr.List())
	})

	// Register version handler
	server.Register("version", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(version.Get())
	})
}
