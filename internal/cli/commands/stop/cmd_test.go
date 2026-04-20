package stop_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/stop"
)

// mockClient satisfies transport.IPCClient without a live daemon.
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

func TestRun_MissingArgs(t *testing.T) {
	err := stop.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success_WasRunning(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{
			"status":      "stopped",
			"id":          "abc-123",
			"was_running": true,
		},
	}
	err := stop.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "stop" {
		t.Errorf("expected one 'stop' call, got %v", mc.calls)
	}
}

func TestRun_Success_AlreadyStopped(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{
			"status":      "stopped",
			"id":          "abc-123",
			"was_running": false,
		},
	}
	// Should not return an error even when already stopped
	err := stop.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("connection refused")}
	// Per-target errors are printed as they happen but Run returns a
	// non-nil error so scripts see a non-zero exit code.
	err := stop.Run(mc, []string{"abc-123"})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestRun_MultipleIDs(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{
			"status":      "stopped",
			"id":          "x",
			"was_running": true,
		},
	}
	err := stop.Run(mc, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 3 {
		t.Errorf("expected 3 IPC calls, got %d", len(mc.calls))
	}
}

func TestGetSpec(t *testing.T) {
	spec := stop.GetSpec()
	if spec.Name != "stop" {
		t.Errorf("expected name 'stop', got %s", spec.Name)
	}
	if spec.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestRun_JSONOutput(t *testing.T) {
	calls := 0
	mc := &scriptedMock{fn: func(cmd string, _ any, result any) error {
		calls++
		var resp map[string]any
		switch calls {
		case 1:
			resp = map[string]any{"status": "stopped", "id": "running-proc", "was_running": true}
		case 2:
			resp = map[string]any{"status": "stopped", "id": "already-stopped", "was_running": false}
		case 3:
			return errors.New("not found")
		}
		b, _ := json.Marshal(resp)
		_ = json.Unmarshal(b, result)
		return nil
	}}

	got := captureStdout(t, func() {
		err := stop.Run(mc, []string{"a", "b", "c", "--json"})
		if err == nil {
			t.Error("expected non-nil error from 1 failed target")
		}
	})

	var decoded struct {
		Op      string `json:"op"`
		Results []struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Error  string         `json:"error,omitempty"`
			Extra  map[string]any `json:"extra,omitempty"`
		} `json:"results"`
		Summary struct {
			Total  int `json:"total"`
			Ok     int `json:"ok"`
			Failed int `json:"failed"`
			Noop   int `json:"noop"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, got)
	}
	if decoded.Op != "stop" {
		t.Errorf("op=%q, want 'stop'", decoded.Op)
	}
	if decoded.Summary.Total != 3 || decoded.Summary.Ok != 1 ||
		decoded.Summary.Noop != 1 || decoded.Summary.Failed != 1 {
		t.Errorf("summary = %+v", decoded.Summary)
	}
	// Sanity: in --json mode, nothing human-readable should appear before
	// the JSON payload.
	trimmed := strings.TrimSpace(got)
	if !strings.HasPrefix(trimmed, "{") {
		t.Errorf("expected output to be pure JSON, got:\n%s", got)
	}
}

func TestRun_PartialFailure(t *testing.T) {
	calls := 0
	mc := &scriptedMock{fn: func(cmd string, _ any, result any) error {
		calls++
		if calls == 2 {
			return errors.New("boom")
		}
		b, _ := json.Marshal(map[string]any{"status": "stopped", "id": "x", "was_running": true})
		_ = json.Unmarshal(b, result)
		return nil
	}}
	err := stop.Run(mc, []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected aggregate error")
	}
	if !strings.Contains(err.Error(), "1 of 3") {
		t.Errorf("expected '1 of 3 targets failed', got: %v", err)
	}
}

type scriptedMock struct {
	fn func(cmd string, params, result any) error
}

func (m *scriptedMock) Call(cmd string, p any, r any) error { return m.fn(cmd, p, r) }
func (m *scriptedMock) Close() error                        { return nil }

func TestRun_NamespaceFlag_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
		{"id": "id-dev-api", "name": "api", "namespace": "dev"},
	}
	stops := []string{}
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "stop":
			b, _ := json.Marshal(map[string]any{"status": "stopped", "id": "x", "was_running": true})
			_ = json.Unmarshal(b, result)
			stops = append(stops, "stop")
		}
		return nil
	}}
	if err := stop.Run(mc, []string{"--namespace", "prod"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(stops) != 2 {
		t.Errorf("expected 2 stops for namespace prod, got %d", len(stops))
	}
}

func TestRun_NSWildcard_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
		{"id": "id-dev-api", "name": "api", "namespace": "dev"},
	}
	stops := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "stop":
			stops++
			b, _ := json.Marshal(map[string]any{"status": "stopped", "id": "x", "was_running": true})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := stop.Run(mc, []string{"prod:*"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if stops != 2 {
		t.Errorf("expected 2 stops, got %d", stops)
	}
}

func TestRun_NamespaceFlag_RejectsMixWithPositional(t *testing.T) {
	mc := &mockClient{response: map[string]any{}}
	err := stop.Run(mc, []string{"api", "--namespace", "prod"})
	if err == nil {
		t.Fatal("expected usage error mixing --namespace with positional")
	}
	if !strings.Contains(err.Error(), "cannot combine --namespace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_NamespaceFlag_EmptyNamespaceErrors(t *testing.T) {
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		if cmd == "list" {
			b, _ := json.Marshal([]map[string]any{})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	err := stop.Run(mc, []string{"--namespace", "ghost"})
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if !strings.Contains(err.Error(), `"ghost"`) {
		t.Errorf("err = %v, want mention of 'ghost'", err)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever was written. Batch reports go straight to os.Stdout via jsonx.
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
