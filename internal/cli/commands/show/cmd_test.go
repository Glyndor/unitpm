package show_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/show"
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

func TestRun_MissingArgs(t *testing.T) {
	err := show.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{
			"info": map[string]any{
				"id":           "abc-123",
				"name":         "myapp",
				"namespace":    "default",
				"state":        "running",
				"pid":          1234,
				"cpu":          1.5,
				"memory_bytes": 1024 * 1024,
				"uptime_ms":    60000,
				"restarts":     2,
				"user":         "root",
				"mode":         "self",
				"version":      "1",
			},
		},
	}
	err := show.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "show" {
		t.Errorf("expected one 'show' call, got %v", mc.calls)
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("process not found")}
	err := show.Run(mc, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error from IPC failure")
	}
	if !strings.Contains(err.Error(), "show failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := show.GetSpec()
	if spec.Name != "show" {
		t.Errorf("expected name 'show', got %s", spec.Name)
	}
}
