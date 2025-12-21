package start

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestParseStartSpec(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    protocol.StartSpec
		wantErr bool
		errCode string
	}{
		{
			name: "lynx start main.js",
			args: []string{"main.js"},
			want: protocol.StartSpec{
				Cmd:   "main.js",
				Args:  []string{},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "lynx start main.go --name Test",
			args: []string{"main.go", "--name", "Test"},
			want: protocol.StartSpec{
				Name:  "Test",
				Cmd:   "main.go",
				Args:  []string{},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "lynx start \"bun dev\"",
			args: []string{"bun dev"},
			want: protocol.StartSpec{
				Cmd:   "bun",
				Args:  []string{"dev"},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "lynx start \"node --run dev\" --name test",
			args: []string{"node --run dev", "--name", "test"},
			want: protocol.StartSpec{
				Name:  "test",
				Cmd:   "node",
				Args:  []string{"--run", "dev"},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "lynx start node --run dev",
			args: []string{"node", "--run", "dev"},
			want: protocol.StartSpec{
				Cmd:   "node",
				Args:  []string{"--run", "dev"},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "lynx start -- node --run dev",
			args: []string{"--", "node", "--run", "dev"},
			want: protocol.StartSpec{
				Cmd:   "node",
				Args:  []string{"--run", "dev"},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name:    "lynx start node --cron ...",
			args:    []string{"node", "--cron", "0 1 * * *"},
			wantErr: true,
			errCode: "ERR_UNSUPPORTED",
		},
		// Additional edge cases
		{
			name: "flag after command",
			args: []string{"sleep", "10", "--name", "sleeper"},
			want: protocol.StartSpec{
				Name:  "sleeper",
				Cmd:   "sleep",
				Args:  []string{"10"},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "mixed flags and args",
			args: []string{"--name", "test", "cmd", "--flag", "arg"},
			want: protocol.StartSpec{
				Name:  "test",
				Cmd:   "cmd",
				Args:  []string{"--flag", "arg"},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{Mode: "self"},
			},
			wantErr: false,
		},
		{
			name: "explicit user",
			args: []string{"--run-as", "explicit_user", "--username", "nobody", "whoami"},
			want: protocol.StartSpec{
				Cmd:   "whoami",
				Args:  []string{},
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{
					Mode:     "explicit_user",
					Username: "nobody",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStartSpec(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStartSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if tt.errCode != "" && !strings.Contains(err.Error(), tt.errCode) {
					t.Errorf("parseStartSpec() error = %v, want code %v", err, tt.errCode)
				}
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("parseStartSpec() = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		{
			input: "bun dev",
			want:  []string{"bun", "dev"},
		},
		{
			input: "node --run dev",
			want:  []string{"node", "--run", "dev"},
		},
		{
			input: "'single quoted'",
			want:  []string{"single quoted"},
		},
		{
			input: "\"double quoted\"",
			want:  []string{"double quoted"},
		},
		{
			input: "escaped\\ space",
			want:  []string{"escaped space"},
		},
		{
			input: "\"escaped \\\" quote\"",
			want:  []string{"escaped \" quote"},
		},
		{
			input:   "unclosed quote '",
			wantErr: true,
		},
		{
			input:   "trailing backslash \\",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := tokenize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("tokenize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("tokenize() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
