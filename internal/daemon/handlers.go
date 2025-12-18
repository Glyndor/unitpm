package daemon

import (
	"encoding/json"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/version"
)

// RegisterHandlers registers all daemon IPC handlers.
func RegisterHandlers(server *ipc.Server, mgr *Manager) {
	// Register ping handler
	server.Register("ping", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	// Register start handler
	server.Register("start", func(params json.RawMessage) (json.RawMessage, error) {
		var args struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.Start(args.Name, args.Command)
		if err != nil {
			return nil, err
		}

		return json.Marshal(map[string]int{"id": id})
	})

	// Register stop handler
	server.Register("stop", func(params json.RawMessage) (json.RawMessage, error) {
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
	server.Register("list", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(mgr.List())
	})

	// Register version handler
	server.Register("version", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(version.Get())
	})
}
