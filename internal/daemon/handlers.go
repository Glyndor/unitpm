// Package daemon provides the core daemon logic and initialization.
package daemon

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/spec"
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

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			return nil, err
		}

		if err := mgr.Stop(id); err != nil {
			return nil, err
		}

		return jsonx.Marshal(map[string]string{"status": "stopped", "id": id})
	})

	// Register restart handler
	server.Register("restart", func(
		_ context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID string `json:"id"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			return nil, err
		}

		if err := mgr.Restart(id); err != nil {
			return nil, err
		}

		return jsonx.Marshal(map[string]string{"status": "restarted", "id": id})
	})

	// Register delete handler
	server.Register("delete", func(
		_ context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID    string `json:"id"`
			Purge bool   `json:"purge"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			return nil, err
		}

		// Prepare for purge (resolve log dir)
		var appLogDir string
		if args.Purge {
			if proc, ok := mgr.Get(id); ok {
				// Re-implement log dir logic briefly
				s := proc.Spec()
				configuredDir := ""
				if s.Logs != nil {
					configuredDir = s.Logs.Dir
				}
				
				var baseLogDir string
				if configuredDir != "" {
					baseLogDir = configuredDir
				} else if os.Geteuid() == 0 {
					baseLogDir = "/var/log/lynx"
				} else if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
					baseLogDir = filepath.Join(stateHome, "lynx/logs")
				} else if home, err := os.UserHomeDir(); err == nil {
					baseLogDir = filepath.Join(home, ".local/state/lynx/logs")
				}
				
				if baseLogDir != "" {
					appLogDir = filepath.Join(baseLogDir, id)
				}
			}
		}

		if err := mgr.Delete(id); err != nil {
			return nil, err
		}

		// Delete spec
		_ = spec.DeleteSpec(id) // Ignore error if spec missing

		// Delete logs if requested
		if args.Purge && appLogDir != "" {
			_ = os.RemoveAll(appLogDir)
		}

		// Delete credentials if dynamic user
		credsDir := filepath.Join("/var/lib/lynx/creds", id)
		_ = os.RemoveAll(credsDir)

		return jsonx.Marshal(map[string]string{"status": "deleted", "id": id})
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
