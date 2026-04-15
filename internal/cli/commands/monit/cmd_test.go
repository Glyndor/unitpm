package monit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/monit"
)

type mockClient struct {
	err   error
	calls int
}

func (m *mockClient) Call(_ string, _ any, _ any) error {
	m.calls++
	return m.err
}

func (m *mockClient) Close() error { return nil }

func TestGetSpec(t *testing.T) {
	spec := monit.GetSpec()
	if spec.Name != "monit" {
		t.Errorf("expected name 'monit', got %s", spec.Name)
	}
	if spec.Description == "" {
		t.Error("expected non-empty description")
	}
	if spec.Usage == "" {
		t.Error("expected non-empty usage")
	}
}

func TestRun_IPCError(t *testing.T) {
	// monit loops forever on success; only returns on IPC error
	mc := &mockClient{err: errors.New("connection refused")}
	err := monit.Run(mc, []string{})
	if err == nil {
		t.Fatal("expected error from IPC failure")
	}
	if !strings.Contains(err.Error(), "monit failed") {
		t.Errorf("unexpected error: %v", err)
	}
	if mc.calls != 1 {
		t.Errorf("expected 1 IPC call before error, got %d", mc.calls)
	}
}
