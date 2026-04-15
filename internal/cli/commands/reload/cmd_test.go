package reload_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/reload"
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
	err := reload.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "reloaded", "id": "abc-123"},
	}
	err := reload.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "reload" {
		t.Errorf("expected one 'reload' call, got %v", mc.calls)
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("connection refused")}
	err := reload.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Errorf("expected nil (errors printed not returned), got %v", err)
	}
}

func TestRun_MultipleIDs(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "reloaded", "id": "x"},
	}
	err := reload.Run(mc, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(mc.calls))
	}
}

func TestGetSpec(t *testing.T) {
	spec := reload.GetSpec()
	if spec.Name != "reload" {
		t.Errorf("expected name 'reload', got %s", spec.Name)
	}
}
