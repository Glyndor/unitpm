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
	// Per-target errors are printed as they happen; Run returns a
	// non-nil error so scripts get a non-zero exit code.
	err := reset.Run(mc, []string{"ghost"})
	if err == nil {
		t.Fatal("expected non-nil error when target fails")
	}
	if !strings.Contains(err.Error(), "reset") {
		t.Errorf("expected error to mention op, got: %v", err)
	}
}

func TestRun_PartialFailure(t *testing.T) {
	// One call succeeds, one fails → returns aggregate error.
	calls := 0
	mc := &failingMockClient{
		fn: func(cmd string, _ any, result any) error {
			calls++
			if calls == 2 {
				return errors.New("boom")
			}
			b, _ := json.Marshal(map[string]any{"status": "reset", "id": "ok-id"})
			_ = json.Unmarshal(b, result)
			return nil
		},
	}
	err := reset.Run(mc, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected aggregate error")
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("expected '1 of 2 targets failed', got: %v", err)
	}
}

type failingMockClient struct {
	fn func(cmd string, params, result any) error
}

func (m *failingMockClient) Call(cmd string, p any, r any) error { return m.fn(cmd, p, r) }
func (m *failingMockClient) Close() error                        { return nil }

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

func TestRun_NamespaceFlag_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
		{"id": "id-dev-api", "name": "api", "namespace": "dev"},
	}
	resets := 0
	mc := &failingMockClient{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "reset":
			resets++
			b, _ := json.Marshal(map[string]any{"status": "reset", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := reset.Run(mc, []string{"--namespace", "prod"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if resets != 2 {
		t.Errorf("expected 2 resets, got %d", resets)
	}
}

func TestRun_NSWildcard_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
	}
	resets := 0
	mc := &failingMockClient{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "reset":
			resets++
			b, _ := json.Marshal(map[string]any{"status": "reset", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := reset.Run(mc, []string{"prod:*"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if resets != 2 {
		t.Errorf("expected 2 resets, got %d", resets)
	}
}

func TestRun_NamespaceFlag_RejectsMixWithPositional(t *testing.T) {
	mc := &mockClient{response: map[string]any{}}
	err := reset.Run(mc, []string{"api", "--namespace", "prod"})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --namespace") {
		t.Errorf("err = %v", err)
	}
}

func TestRun_NamespaceFlag_EmptyNamespaceErrors(t *testing.T) {
	mc := &failingMockClient{fn: func(cmd string, _, result any) error {
		if cmd == "list" {
			b, _ := json.Marshal([]map[string]any{})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := reset.Run(mc, []string{"--namespace", "ghost"}); err == nil {
		t.Fatal("expected empty-namespace error")
	}
}
