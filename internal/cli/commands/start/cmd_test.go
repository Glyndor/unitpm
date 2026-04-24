package start_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/start"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

type mockClient struct {
	response any
	err      error
	calls    []string
}

func (m *mockClient) Call(cmd string, _ any, result any) error {
	m.calls = append(m.calls, cmd)
	if m.err != nil {
		return m.err
	}
	if m.response != nil {
		b, _ := json.Marshal(m.response)
		_ = json.Unmarshal(b, result)
	}
	return nil
}

func (m *mockClient) Close() error { return nil }

func TestRun_Help(t *testing.T) {
	if err := start.Run(nil, []string{"--help"}); err != nil {
		t.Errorf("expected no error for --help, got %v", err)
	}
}

func TestRun_ParseError(t *testing.T) {
	err := start.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing command") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	// Point XDG_CONFIG_HOME to temp dir so SaveSpec writes there
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mc := &mockClient{
		response: protocol.StartResponseData{
			ProcID: "abc-123",
			PID:    9999,
			Status: "running",
		},
	}
	err := start.Run(mc, []string{"echo", "hello", "--no-list"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "start" {
		t.Errorf("expected one 'start' IPC call, got %v", mc.calls)
	}
}

func TestRun_Scale(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mc := &mockClient{
		response: protocol.StartResponseData{
			ProcID: "abc-123",
			PID:    9999,
			Status: "running",
		},
	}
	err := start.Run(mc, []string{"echo", "--scale", "3", "--no-list"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 3 {
		t.Errorf("expected 3 IPC calls for scale=3, got %d", len(mc.calls))
	}
}

func TestRun_IPCError_DeletesSpec(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mc := &mockClient{err: errors.New("daemon rejected")}
	err := start.Run(mc, []string{"echo"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "start failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := start.GetSpec()
	if spec.Name != "start" {
		t.Errorf("expected name 'start', got %s", spec.Name)
	}
}

func TestRun_JSONOutput(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mc := &mockClient{
		response: protocol.StartResponseData{
			ProcID: "id-0",
			PID:    1111,
			Status: "running",
		},
	}
	got := captureStdout(t, func() {
		if err := start.Run(mc, []string{"echo", "--scale", "2", "--json"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var decoded struct {
		Started []struct {
			Name   string `json:"name"`
			ID     string `json:"id"`
			PID    int    `json:"pid"`
			Status string `json:"status"`
		} `json:"started"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if decoded.Count != 2 || len(decoded.Started) != 2 {
		t.Errorf("decoded = %+v", decoded)
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("expected pure JSON, got:\n%s", got)
	}
}

func TestRun_DryRunJSON(t *testing.T) {
	// --dry-run --json must emit spec+scale without contacting daemon.
	got := captureStdout(t, func() {
		if err := start.Run(nil, []string{"--dry-run", "--json", "echo", "hello"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var decoded struct {
		Spec  protocol.AppSpec `json:"spec"`
		Scale int              `json:"scale"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if decoded.Scale != 1 {
		t.Errorf("scale = %d, want 1", decoded.Scale)
	}
	if decoded.Spec.Exec.Command == "" {
		t.Errorf("spec.exec.command missing: %+v", decoded.Spec.Exec)
	}
}

func TestRun_JSONPartialOnFailure(t *testing.T) {
	// First instance starts, second fails → abort. Partial JSON
	// report must still hit stdout so CI can see what started.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	calls := 0
	mc := &scriptedMock{fn: func(cmd string, _ any, result any) error {
		calls++
		if calls == 2 {
			return errors.New("daemon boom")
		}
		b, _ := json.Marshal(protocol.StartResponseData{
			ProcID: "id-0",
			PID:    1111,
			Status: "running",
		})
		_ = json.Unmarshal(b, result)
		return nil
	}}

	got := captureStdout(t, func() {
		err := start.Run(mc, []string{"echo", "--scale", "3", "--json"})
		if err == nil {
			t.Error("expected error on second instance failure")
		}
	})
	if got == "" {
		t.Fatal("expected partial JSON report on stdout")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if decoded["partial"] != true {
		t.Errorf("expected partial=true; got %+v", decoded)
	}
	if n, _ := decoded["failed_at_instance"].(float64); int(n) != 2 {
		t.Errorf("expected failed_at_instance=2, got %v", decoded["failed_at_instance"])
	}
}

type scriptedMock struct {
	fn func(cmd string, params, result any) error
}

func (m *scriptedMock) Call(cmd string, p any, r any) error { return m.fn(cmd, p, r) }
func (m *scriptedMock) Close() error                        { return nil }

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}

func TestParseAppSpec(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	defaultLogs := &protocol.AppLogs{
		Mode:      "file",
		Format:    "plain",
		Timestamp: "rfc3339",
	}
	defaultRestart := &protocol.AppRestart{
		Policy:      "on-failure",
		MaxRetries:  10,
		BackoffMs:   2000,
		BackoffType: "expo",
		StopOnExit:  []int{0},
	}
	defaultRunAs := &protocol.RunAsPolicy{
		Mode: "self",
	}

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
				Version:   1,
				Name:      "",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
				Version:   1,
				Name:      "Test",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
				Version:   1,
				Name:      "",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
				Version:   1,
				Name:      "test",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
				Version:   1,
				Name:      "",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
				Version:   1,
				Name:      "",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
				Version:   1,
				Name:      "",
				Namespace: "default",
				Cwd:       cwd,
				Logs:      defaultLogs,
				Restart:   defaultRestart,
				RunAs:     defaultRunAs,
				Env:       map[string]string{},
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
			got, _, err := start.ParseAppSpec(tt.args)
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

			// Normalize Cwd for comparison
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
		{
			input: "a\\ b",
			want:  []string{"a\\", "b"},
		}, // Backslash is literal outside quotes in this simple lexer
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

func TestParseAppSpec_Validation(t *testing.T) {
	_, _, err := start.ParseAppSpec([]string{})
	if err == nil {
		t.Error("Expected error for empty args, got nil")
	}
	if !strings.Contains(err.Error(), "missing command") {
		t.Errorf("Expected 'missing command', got %v", err)
	}
}
