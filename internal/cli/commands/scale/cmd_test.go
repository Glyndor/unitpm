package scale_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/scale"
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
	err := scale.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("unexpected error: %v", err)
	}
	if err := scale.Run(nil, []string{"onlyname"}); err == nil {
		t.Fatal("expected usage error with only one arg")
	}
}

func TestRun_BadCount(t *testing.T) {
	for _, bad := range []string{"abc", "-1", "1.5"} {
		err := scale.Run(nil, []string{"worker", bad})
		if err == nil {
			t.Errorf("target %q should be rejected", bad)
		}
	}
}

func TestRun_Help(t *testing.T) {
	if err := scale.Run(nil, []string{"--help"}); err != nil {
		t.Errorf("--help returned error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{
			"base_name": "worker",
			"namespace": "default",
			"before":    2,
			"after":     5,
			"created":   []string{"worker-3", "worker-4", "worker-5"},
		},
	}
	if err := scale.Run(mc, []string{"worker", "5"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "scale" {
		t.Errorf("expected one 'scale' call, got %v", mc.calls)
	}
	// Ensure the request included the parsed name+namespace+target.
	b, _ := json.Marshal(mc.params[0])
	if !strings.Contains(string(b), `"name":"worker"`) ||
		!strings.Contains(string(b), `"target":5`) {
		t.Errorf("unexpected params: %s", b)
	}
}

func TestRun_NamespaceQualified(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"base_name": "api", "namespace": "prod", "before": 1, "after": 3},
	}
	if err := scale.Run(mc, []string{"prod:api", "3"}); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(mc.params[0])
	if !strings.Contains(string(b), `"name":"api"`) ||
		!strings.Contains(string(b), `"namespace":"prod"`) {
		t.Errorf("namespace not parsed: %s", b)
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("not found")}
	err := scale.Run(mc, []string{"worker", "2"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "scale failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := scale.GetSpec()
	if spec.Name != "scale" {
		t.Errorf("expected name 'scale', got %s", spec.Name)
	}
}

func TestPrintHelp(t *testing.T) {
	scale.PrintHelp()
}
