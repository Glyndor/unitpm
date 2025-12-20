package start

import (
	"reflect"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestParseStartSpec(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    protocol.StartSpec
		wantErr bool
	}{
		{
			name: "basic command",
			args: []string{"ls", "-la"},
			want: protocol.StartSpec{
				Name:  "",
				Cmd:   "ls",
				Args:  []string{"-la"},
				Cwd:   "",
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{
					Mode: "self",
				},
			},
			wantErr: false,
		},
		{
			name: "with flags",
			args: []string{"--name", "testproc", "--cwd", "/tmp", "--env", "FOO=bar", "--", "sleep", "10"},
			want: protocol.StartSpec{
				Name:  "testproc",
				Cmd:   "sleep",
				Args:  []string{"10"},
				Cwd:   "/tmp",
				Env:   map[string]string{"FOO": "bar"},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{
					Mode: "self",
				},
			},
			wantErr: false,
		},
		{
			name: "explicit user",
			args: []string{"--run-as", "explicit_user", "--username", "nobody", "whoami"},
			want: protocol.StartSpec{
				Name:  "",
				Cmd:   "whoami",
				Args:  []string{},
				Cwd:   "",
				Env:   map[string]string{},
				Stdio: "inherit",
				RunAs: protocol.RunAsPolicy{
					Mode:     "explicit_user",
					Username: "nobody",
				},
			},
			wantErr: false,
		},
		{
			name:    "missing command",
			args:    []string{"--name", "foo"},
			want:    protocol.StartSpec{},
			wantErr: true,
		},
		{
			name:    "missing username for explicit_user",
			args:    []string{"--run-as", "explicit_user", "cmd"},
			want:    protocol.StartSpec{},
			wantErr: true,
		},
		{
			name:    "invalid env format",
			args:    []string{"--env", "INVALID", "cmd"},
			want:    protocol.StartSpec{},
			wantErr: true,
		},
		{
			name:    "invalid stdio",
			args:    []string{"--stdio", "invalid", "cmd"},
			want:    protocol.StartSpec{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStartSpec(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStartSpec() error = %v, wantErr %v", err, tt.wantErr)
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
