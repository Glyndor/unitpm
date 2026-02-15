//go:build linux

package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

// StartHandler handles the start command.
func StartHandler(mgr *manager.Manager, privileged bool) transport.CommandHandler {
	return func(ctx context.Context, params jsonx.RawMessage) (jsonx.RawMessage, error) {
		var req protocol.StartRequest
		if err := jsonx.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: %w", err)
		}

		spec := req.Spec
		if spec.ID == "" {
			// If for some reason ID is missing (legacy?), we might need to gen one,
			// but for v1 we expect it.
			return nil, errors.New("ERR_BAD_REQUEST: spec ID is required")
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
			ProcID:    procInfo.ID,
			PID:       procInfo.PID,
			Status:    string(procInfo.State),
			CreatedAt: time.Now().Format(time.RFC3339),
		}

		return jsonx.Marshal(respData)
	}
}
