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
			return nil, errors.New("ERR_BAD_REQUEST: spec ID is required")
		}

		// Peer identity (uid/gid/pid) comes from SO_PEERCRED — the IPC
		// server attaches it to ctx for every request. Future per-user
		// isolation modes (explicit_user, app_user) will consult it.
		identity, ok := ctx.Value(transport.ContextKeyIdentity).(*transport.Identity)
		if !ok {
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
