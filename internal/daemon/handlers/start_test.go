//go:build linux

package handlers_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

func TestStartHandler_Validation(t *testing.T) {
	mgr := manager.NewManager()
	handler := handlers.StartHandler(mgr, false) // unprivileged

	// Context with identity
	ctx := context.WithValue(
		context.Background(),
		transport.ContextKeyIdentity,
		&transport.Identity{
			UID: "1000",
			GID: "1000",
			PID: 1234,
		},
	)

	tests := []struct {
		name    string
		spec    protocol.AppSpec
		wantErr bool
		errCode string
	}{
		{
			name: "valid self",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174001",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "echo",
					Args:    []string{"hello"},
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "missing exec type",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174002",
				Exec: protocol.AppExec{
					Type: "",
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "missing command",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174003",
				Exec: protocol.AppExec{
					Type: "command",
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "too many args",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174004",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "echo",
					Args:    make([]string, 300),
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "cmd too long",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174005",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: strings.Repeat("a", 4097),
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "env too many",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174006",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "echo",
				},
				Env:   makeEnv(129),
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "invalid name",
			spec: protocol.AppSpec{
				ID:   "123e4567-e89b-12d3-a456-426614174007",
				Name: "Invalid Name!",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "echo",
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "invalid cwd",
			spec: protocol.AppSpec{
				ID:  "123e4567-e89b-12d3-a456-426614174008",
				Cwd: "/path/to/nonexistent/directory",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "echo",
				},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "app_user unsupported",
			spec: protocol.AppSpec{
				ID: "123e4567-e89b-12d3-a456-426614174009",
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "echo",
				},
				RunAs: &protocol.RunAsPolicy{Mode: "app_user"},
			},
			wantErr: true,
			errCode: "ERR_UNSUPPORTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrap in StartRequest
			req := protocol.StartRequest{
				ProtocolVersion: 1,
				Type:            "start",
				RequestID:       tt.spec.ID,
				Spec:            tt.spec,
			}
			reqBytes, err := jsonx.Marshal(req)
			if err != nil {
				t.Fatalf("Failed to marshal req: %v", err)
			}

			_, err = handler(ctx, reqBytes)
			if (err != nil) != tt.wantErr {
				t.Errorf("StartHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errCode != "" && !strings.Contains(err.Error(), tt.errCode) {
					t.Errorf("StartHandler() error = %v, want code %v", err, tt.errCode)
				}
			}
		})
	}
}

func TestStartHandler_Execution(t *testing.T) {
	mgr := manager.NewManager()
	handler := handlers.StartHandler(mgr, false)

	// Context with identity
	ctx := context.WithValue(
		context.Background(),
		transport.ContextKeyIdentity,
		&transport.Identity{
			UID: "1000",
			GID: "1000",
			PID: 1234,
		},
	)

	spec := protocol.AppSpec{
		ID: "123e4567-e89b-12d3-a456-426614174099",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "go", // using 'go' as a command that exists
			Args:    []string{"version"},
		},
		RunAs: &protocol.RunAsPolicy{Mode: "self"},
	}

	req := protocol.StartRequest{
		ProtocolVersion: 1,
		Type:            "start",
		RequestID:       spec.ID,
		Spec:            spec,
	}
	reqBytes, err := jsonx.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal req: %v", err)
	}

	respBytes, err := handler(ctx, reqBytes)
	if err != nil {
		t.Fatalf("StartHandler() failed: %v", err)
	}

	var respData protocol.StartResponseData
	if err := jsonx.Unmarshal(respBytes, &respData); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if respData.ProcID != spec.ID {
		t.Errorf("ProcID = %s, want %s", respData.ProcID, spec.ID)
	}
	if respData.PID <= 0 {
		t.Errorf("PID = %d, want > 0", respData.PID)
	}
}

func makeEnv(count int) map[string]string {
	env := make(map[string]string)
	for i := 0; i < count; i++ {
		// Better:
		key := "k" + strconv.Itoa(i)
		env[key] = "val"
	}
	return env
}
