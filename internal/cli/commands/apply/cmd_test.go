package apply_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/apply"
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

func writeTempLynxfile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "Lynxfile-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

const validLynxfile = `
version: "1"
namespace: test
apps:
  - name: echo-app
    command: echo hello
    cwd: /tmp
`

const invalidLynxfile = `
this: is: not: valid: yaml: :::
`

func TestRun_MissingArgs(t *testing.T) {
	err := apply.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing Lynxfile path") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Help(t *testing.T) {
	err := apply.Run(nil, []string{"--help"})
	if err != nil {
		t.Errorf("expected no error for --help, got %v", err)
	}
}

func TestRun_FileNotFound(t *testing.T) {
	err := apply.Run(nil, []string{"/nonexistent/path/Lynxfile.yml"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_InvalidYAML(t *testing.T) {
	path := writeTempLynxfile(t, invalidLynxfile)
	err := apply.Run(nil, []string{path})
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestRun_Success(t *testing.T) {
	path := writeTempLynxfile(t, validLynxfile)

	mc := &mockClient{
		response: protocol.StartResponseData{
			ProcID: "test-id-123",
			PID:    9999,
			Status: "running",
		},
	}

	// apply saves specs to disk; clean up the spec dir after
	err := apply.Run(mc, []string{path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) < 1 || mc.calls[0] != "start" {
		t.Errorf("expected 'start' IPC call, got %v", mc.calls)
	}
}

func TestRun_IPCError(t *testing.T) {
	path := writeTempLynxfile(t, validLynxfile)

	mc := &mockClient{err: errors.New("daemon unavailable")}
	err := apply.Run(mc, []string{path})
	if err == nil {
		t.Fatal("expected error from IPC failure")
	}
	if !strings.Contains(err.Error(), "apply failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_JSONOutput(t *testing.T) {
	path := writeTempLynxfile(t, validLynxfile)
	mc := &mockClient{response: protocol.StartResponseData{
		ProcID: "id-1",
		PID:    1234,
		Status: "running",
	}}

	got := captureStdout(t, func() {
		if err := apply.Run(mc, []string{path, "--json"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var decoded struct {
		Op      string `json:"op"`
		Results []struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Extra  map[string]any `json:"extra,omitempty"`
		} `json:"results"`
		Summary struct {
			Total, Ok, Failed int
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if decoded.Op != "apply" {
		t.Errorf("op=%q", decoded.Op)
	}
	if decoded.Summary.Total != 1 || decoded.Summary.Ok != 1 {
		t.Errorf("summary = %+v", decoded.Summary)
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("expected pure JSON, got:\n%s", got)
	}
}

func TestRun_JSONOutputPartialOnAbort(t *testing.T) {
	path := writeTempLynxfile(t, validLynxfile)
	mc := &mockClient{err: errors.New("daemon unavailable")}

	got := captureStdout(t, func() {
		_ = apply.Run(mc, []string{path, "--json"})
	})
	// Even on abort, --json must emit something structured so CI
	// scripts know which apps were applied before the failure.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("expected JSON even on abort; got:\n%s\nerr: %v", got, err)
	}
	// Partial-at-first-spec: shape is the batch report with summary.failed>=1.
	if sum, ok := decoded["summary"].(map[string]any); ok {
		if failed, _ := sum["failed"].(float64); failed < 1 {
			t.Errorf("expected summary.failed >= 1, got %v", sum)
		}
	}
}

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
