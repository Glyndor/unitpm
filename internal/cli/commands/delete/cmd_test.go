package deletecmd_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	deletecmd "github.com/Jaro-c/Lynx/internal/cli/commands/delete"
)

type mockClient struct {
	response any
	err      error
	calls    []string
	params   []any
}

func (m *mockClient) Call(cmd string, params any, result any) error {
	m.calls = append(m.calls, cmd)
	m.params = append(m.params, params)
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
	err := deletecmd.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_OnlyFlags(t *testing.T) {
	err := deletecmd.Run(nil, []string{"--purge"})
	if err == nil {
		t.Fatal("expected error when only flags provided")
	}
}

func TestRun_Success(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "deleted", "id": "abc-123"},
	}
	err := deletecmd.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "delete" {
		t.Errorf("expected one 'delete' call, got %v", mc.calls)
	}
}

func TestRun_Purge(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "deleted", "id": "abc-123"},
	}
	err := deletecmd.Run(mc, []string{"--purge", "abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Verify purge was set in the request params
	if len(mc.params) < 1 {
		t.Fatal("expected params to be recorded")
	}
	b, _ := json.Marshal(mc.params[0])
	if !strings.Contains(string(b), `"purge":true`) {
		t.Errorf("expected purge=true in params, got %s", string(b))
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("connection refused")}
	// errors printed but Run continues; returns nil
	err := deletecmd.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Errorf("expected nil (errors printed not returned), got %v", err)
	}
}

func TestRun_MultipleIDs(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "deleted", "id": "x"},
	}
	err := deletecmd.Run(mc, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 3 {
		t.Errorf("expected 3 IPC calls, got %d", len(mc.calls))
	}
}

func TestGetSpec(t *testing.T) {
	spec := deletecmd.GetSpec()
	if spec.Name != "delete" {
		t.Errorf("expected name 'delete', got %s", spec.Name)
	}
}
