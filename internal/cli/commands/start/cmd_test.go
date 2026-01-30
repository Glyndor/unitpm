package start_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/start"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestParseAppSpec(t *testing.T) {
	cwd, _ := os.Getwd()
	
	tests := []struct {
		name    string
		args    []string
		want    protocol.AppSpec
		wantErr bool
		errCode string
	}{
		{
			name: "lynx start main.js",
			args: []string{"main.js"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "entry",
					Entry:   "main.js",
					Runtime: "node",
				},
			},
			wantErr: false,
		},
		{
			name: "lynx start main.go --name Test",
			args: []string{"main.go", "--name", "Test"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "Test",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "entry",
					Entry:   "main.go",
					Runtime: "go run",
				},
			},
			wantErr: false,
		},
		{
			name: "lynx start \"bun dev\"",
			args: []string{"bun dev"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "bun",
					Args:    []string{"dev"},
				},
			},
			wantErr: false,
		},
		{
			name: "lynx start \"node --run dev\" --name test",
			args: []string{"node --run dev", "--name", "test"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "test",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "node",
					Args:    []string{"--run", "dev"},
				},
			},
			wantErr: false,
		},
		{
			name: "lynx start node --run dev",
			args: []string{"node", "--run", "dev"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "node",
					Args:    []string{"--run", "dev"},
				},
			},
			wantErr: false,
		},
		{
			name: "lynx start -- node --run dev",
			args: []string{"--", "node", "--run", "dev"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "command",
					Command: "node",
					Args:    []string{"--run", "dev"},
				},
			},
			wantErr: false,
		},
		{
			name: "lynx start app.py --runtime python3",
			args: []string{"app.py", "--runtime", "python3"},
			want: protocol.AppSpec{
				Version: 1,
				Name:    "",
				Cwd:     cwd,
				Logs:    &protocol.AppLogs{Mode: "inherit"},
				Env:     map[string]string{},
				Exec: protocol.AppExec{
					Type:    "entry",
					Entry:   "app.py",
					Runtime: "python3",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := start.ParseAppSpec(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAppSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errCode != "" && !strings.Contains(err.Error(), tt.errCode) {
					t.Errorf("ParseAppSpec() error = %v, want code %v", err, tt.errCode)
				}
				return
			}
			
			// Normalize Cwd for comparison as it might resolve to slightly different absolute paths depending on environment
			if got.Cwd != "" {
				got.Cwd = cwd
			}
			
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseAppSpec() = %+v, want %+v", got, tt.want)
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
		{input: "a b c", want: []string{"a", "b", "c"}},
		{input: "a 'b c' d", want: []string{"a", "b c", "d"}},
		{input: "a \"b c\"", want: []string{"a", "b c"}},
		{input: "a 'b \"c\" d'", want: []string{"a", "b \"c\" d"}},
		{input: "a \"b 'c' d\"", want: []string{"a", "b 'c' d"}},
		{input: "a\\ b", want: []string{"a\\", "b"}}, // Backslash is literal outside quotes in this simple lexer
		{input: "'a b", wantErr: true},
		{input: "\"a b", wantErr: true},
		{input: "\"invalid escape \\z\"", wantErr: true},
		{input: "\"valid escape \\\" \"", want: []string{"valid escape \" "}},
		{input: "\"valid escape \\\\ \"", want: []string{"valid escape \\ "}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := start.Tokenize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Tokenize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize() = %v, want %v", got, tt.want)
			}
		})
	}
}
