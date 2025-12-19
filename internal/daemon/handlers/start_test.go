package handlers

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestStartHandler_Validation(t *testing.T) {
	mgr := manager.NewManager()
	handler := StartHandler(mgr, false) // unprivileged

	tests := []struct {
		name      string
		spec      protocol.StartSpec
		wantErr   bool
		errCode   string
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
			name: "app_user unprivileged",
			spec: protocol.StartSpec{
				Cmd:   "echo",
				RunAs: protocol.RunAsPolicy{Mode: "app_user"},
			},
			wantErr: true,
			errCode: "ERR_FORBIDDEN",
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
			params, _ := json.Marshal(tt.spec)
			_, err := handler(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("StartHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.errCode) {
					t.Errorf("StartHandler() error = %v, want code %v", err, tt.errCode)
				}
			}
		})
	}
}

func TestStartHandler_Execution(t *testing.T) {
	mgr := manager.NewManager()
	handler := StartHandler(mgr, false)

	var cmd string
	var args []string

	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"-n", "2", "127.0.0.1"}
	} else {
		cmd = "sleep"
		args = []string{"0.1"}
	}

	spec := protocol.StartSpec{
		Cmd:   cmd,
		Args:  args,
		RunAs: protocol.RunAsPolicy{Mode: "self"},
	}

	params, _ := json.Marshal(spec)
	res, err := handler(params)
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
	m := make(map[string]string)
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("K%d", i)] = "v"
	}
	return m
}
