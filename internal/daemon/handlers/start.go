package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// StartHandler handles the start command.
func StartHandler(mgr *manager.Manager, privileged bool) transport.CommandHandler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var spec protocol.StartSpec
		if err := json.Unmarshal(params, &spec); err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: %w", err)
		}

		// TODO: Validate identity from ctx if needed for explicit_user
		identity, ok := ctx.Value(transport.ContextKeyIdentity).(*transport.Identity)
		if !ok {
			// Should not happen if server logic is correct
			return nil, errors.New("INTERNAL_ERROR: identity not found")
		}

		// Start process via Daemon logic
		procInfo, err := StartProcess(mgr, spec, identity, privileged)
		if err != nil {
			return nil, err
		}

		respData := protocol.StartResponseData{
			ProcID:    strconv.Itoa(procInfo.ID),
			PID:       procInfo.PID,
			Status:    string(procInfo.State),
			CreatedAt: time.Now().Format(time.RFC3339),
		}

		return json.Marshal(respData)
	}
}
