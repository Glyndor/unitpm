package policy_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/policy"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

func TestAuthorizeStart(t *testing.T) {
	// Mocks
	identity := &transport.Identity{UID: "1000", GID: "1000", PID: 1234}

	tests := []struct {
		name       string
		spec       protocol.AppSpec
		privileged bool
		wantErr    bool
		errCode    string
	}{
		{
			name: "Simple self run allowed",
			spec: protocol.AppSpec{
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			privileged: false,
			wantErr:    false,
		},
		{
			name: "Shell execution denied for privileged daemon",
			spec: protocol.AppSpec{
				Exec:  protocol.AppExec{Shell: true},
				RunAs: &protocol.RunAsPolicy{Mode: "self"},
			},
			privileged: true,
			wantErr:    true,
			errCode:    "ERR_UNSUPPORTED",
		},
		{
			name: "Dynamic run denied for non-privileged",
			spec: protocol.AppSpec{
				RunAs: &protocol.RunAsPolicy{Mode: "dynamic"},
			},
			privileged: false,
			wantErr:    true,
			errCode:    "ERR_UNSUPPORTED",
		},
		{
			name: "Dynamic run allowed for privileged",
			spec: protocol.AppSpec{
				RunAs: &protocol.RunAsPolicy{Mode: "dynamic"},
			},
			privileged: true,
			wantErr:    false,
		},
		{
			name: "App user not supported",
			spec: protocol.AppSpec{
				RunAs: &protocol.RunAsPolicy{Mode: "app_user"},
			},
			privileged: false,
			wantErr:    true,
			errCode:    "ERR_UNSUPPORTED",
		},
		{
			name: "Explicit user not supported",
			spec: protocol.AppSpec{
				RunAs: &protocol.RunAsPolicy{Mode: "explicit_user"},
			},
			privileged: false,
			wantErr:    true,
			errCode:    "ERR_UNSUPPORTED",
		},
		{
			name: "Invalid mode",
			spec: protocol.AppSpec{
				RunAs: &protocol.RunAsPolicy{Mode: "invalid"},
			},
			privileged: false,
			wantErr:    true,
			errCode:    "ERR_BAD_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := policy.AuthorizeStart(tt.spec, identity, tt.privileged)
			if (err != nil) != tt.wantErr {
				t.Errorf("AuthorizeStart() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errCode != "" && err != nil {
					if err.Error() == "" || err.Error()[:len(tt.errCode)] != tt.errCode {
						t.Errorf("AuthorizeStart() error = %v, want code %v", err, tt.errCode)
					}
				}
			}
		})
	}
}
