//go:build linux

package handlers_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
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
		spec    protocol.StartSpec
		wantErr bool
		errCode string
	}{
		{
			name: "valid self",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Args:  []string{"hello"},
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "missing cmd",
			spec: protocol.StartSpec{
				Args:  []string{"hello"},
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "too many args",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Args:  make([]string, 300),
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "cmd too long",
			spec: protocol.StartSpec{
				Cmd:   strings.Repeat("a", 4097),
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "arg too long",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Args:  []string{strings.Repeat("a", 4097)},
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "env too many",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Env:   makeEnv(129),
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "env key too long",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Env:   map[string]string{strings.Repeat("k", 257): "v"},
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "env value too long",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Env:   map[string]string{"k": strings.Repeat("v", 8193)},
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_LIMITS",
		},
		{
			name: "invalid name",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Name:  "Invalid Name!",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "invalid cwd",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				Cwd:   "/path/to/nonexistent/directory",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: true,
			errCode: "ERR_BAD_REQUEST",
		},
		{
			name: "app_user unsupported",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				RunAs: protocol.RunAsPolicy{Mode: "app_user"},
			},
			wantErr: true,
			errCode: "ERR_UNSUPPORTED",
		},
		{
			name: "explicit_user unsupported",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				RunAs: protocol.RunAsPolicy{Mode: "explicit_user"},
			},
			wantErr: true,
			errCode: "ERR_UNSUPPORTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock request
			reqBytes, err := json.Marshal(tt.spec)
			if err != nil {
				t.Fatalf("Failed to marshal spec: %v", err)
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

	var cmd string
	var args []string

	cmd = "sleep"
	args = []string{"0.1"}

	spec := protocol.StartSpec{
		Cmd:   cmd,
		Args:  args,
		RunAs: protocol.RunAsPolicy{Mode: "self"},
	}

	params, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Failed to marshal spec: %v", err)
	}
	res, err := handler(ctx, params)
	if err != nil {
		t.Fatalf("StartHandler failed: %v", err)
	}

	var data protocol.StartResponseData
	if err := json.Unmarshal(res, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.PID <= 0 {
		t.Errorf("Invalid PID: %d", data.PID)
	}
	t.Logf("Started process PID: %d, Status: %s", data.PID, data.Status)
}

func makeEnv(n int) map[string]string {
	env := make(map[string]string)
	for i := 0; i < n; i++ {
		// Using handlers.MockEnvKey helps if that helper existed, but here we construct manually
		// to avoid circular dependency if MockEnvKey is internal.
		// Since we changed package to handlers_test, we can't access internal helpers unless exported.
		// Assuming makeEnv is local to this test file.
		k := "K" + strings.Repeat("x", i)
		env[k] = "v"
	}
	return env
}
