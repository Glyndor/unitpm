package scale_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
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

func TestRun_JSONOutput(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{
			"base_name": "worker",
			"namespace": "default",
			"before":    2,
			"after":     4,
			"created":   []string{"worker-3", "worker-4"},
		},
	}
	got := captureStdout(t, func() {
		if err := scale.Run(mc, []string{"worker", "4", "--json"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var decoded struct {
		BaseName string   `json:"base_name"`
		Before   int      `json:"before"`
		After    int      `json:"after"`
		Created  []string `json:"created"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if decoded.BaseName != "worker" || decoded.Before != 2 || decoded.After != 4 {
		t.Errorf("decoded = %+v", decoded)
	}
	if len(decoded.Created) != 2 {
		t.Errorf("created len = %d, want 2", len(decoded.Created))
	}
	// --json must be pure JSON, no human lines mixed in.
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("expected pure JSON, got:\n%s", got)
	}
}

func TestRun_FlagAfterPositionals(t *testing.T) {
	mc := &mockClient{response: map[string]any{"base_name": "w", "before": 1, "after": 2}}
	got := captureStdout(t, func() {
		if err := scale.Run(mc, []string{"worker", "2", "--json"}); err != nil {
			t.Fatalf("flag-after-positionals failed: %v", err)
		}
	})
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("--json at the end should still be honored; got:\n%s", got)
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
