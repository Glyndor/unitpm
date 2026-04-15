package reset_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/reset"
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
	err := reset.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "reset", "id": "abc-123"},
	}
	if err := reset.Run(mc, []string{"abc-123"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "reset" {
		t.Errorf("expected one 'reset' call, got %v", mc.calls)
	}
}

func TestRun_MultipleIDs(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "reset", "id": "x"},
	}
	if err := reset.Run(mc, []string{"a", "b", "c"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(mc.calls))
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("not found")}
	// Errors are printed per-ID but Run returns nil.
	if err := reset.Run(mc, []string{"ghost"}); err != nil {
		t.Errorf("expected nil (errors printed not returned), got %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := reset.GetSpec()
	if spec.Name != "reset" {
		t.Errorf("expected name 'reset', got %s", spec.Name)
	}
}

func TestPrintHelp(t *testing.T) {
	// Just ensure it doesn't panic.
	reset.PrintHelp()
}
