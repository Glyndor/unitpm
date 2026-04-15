package stop_test

import (
	"encoding/json"
	"errors"
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
	// IPC errors are printed but Run returns nil (continues for other IDs)
	err := stop.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Errorf("expected nil (errors printed not returned), got %v", err)
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
