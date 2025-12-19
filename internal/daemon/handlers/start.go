package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// StartHandler handles the start command.
func StartHandler(mgr *manager.Manager, privileged bool) transport.CommandHandler {
	return func(params json.RawMessage) (json.RawMessage, error) {
		var spec protocol.StartSpec
		if err := json.Unmarshal(params, &spec); err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: %w", err)
		}

		// Start process via Daemon logic
		procInfo, err := StartProcess(mgr, spec, privileged)
		if err != nil {
			return nil, err
		}
		
		respData := protocol.StartResponseData{
			ProcID:    fmt.Sprintf("%d", procInfo.ID),
			PID:       procInfo.PID,
			Status:    string(procInfo.State),
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		
		return json.Marshal(respData)
	}
}
