package flush_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/flush"
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
	err := flush.Run(nil, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "flushed", "id": "abc-123"},
	}
	err := flush.Run(mc, []string{"abc-123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "flush" {
		t.Errorf("expected one 'flush' call, got %v", mc.calls)
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("connection refused")}
	err := flush.Run(mc, []string{"abc-123"})
	if err == nil {
		t.Fatal("expected non-nil error so scripts see a non-zero exit")
	}
}

func TestRun_MultipleIDs(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "flushed", "id": "x"},
	}
	err := flush.Run(mc, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(mc.calls))
	}
}

func TestGetSpec(t *testing.T) {
	spec := flush.GetSpec()
	if spec.Name != "flush" {
		t.Errorf("expected name 'flush', got %s", spec.Name)
	}
}

func TestRun_BytesFreedSurfaced(t *testing.T) {
	// Daemon now reports bytes_freed; the CLI must pass it through
	// into --json results[].extra.bytes_freed and render it in the
	// human line.
	mc := &mockClient{
		response: map[string]any{
			"status":      "flushed",
			"id":          "abc-123",
			"bytes_freed": 1048576, // 1 MiB
		},
	}
	got := captureStdout(t, func() {
		if err := flush.Run(mc, []string{"--json", "abc-123"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var decoded struct {
		Results []struct {
			ID    string         `json:"id"`
			Extra map[string]any `json:"extra"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if len(decoded.Results) != 1 {
		t.Fatalf("results len = %d", len(decoded.Results))
	}
	bf, ok := decoded.Results[0].Extra["bytes_freed"].(float64)
	if !ok {
		t.Fatalf("extra.bytes_freed missing or not a number: %+v", decoded.Results[0].Extra)
	}
	if int64(bf) != 1048576 {
		t.Errorf("bytes_freed = %d, want 1048576", int64(bf))
	}
}

func TestRun_BytesFreedOmittedWhenZero(t *testing.T) {
	mc := &mockClient{
		response: map[string]any{"status": "flushed", "id": "abc-123"},
	}
	got := captureStdout(t, func() {
		_ = flush.Run(mc, []string{"--json", "abc-123"})
	})
	var decoded struct {
		Results []struct {
			Extra map[string]any `json:"extra"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, has := decoded.Results[0].Extra["bytes_freed"]; has {
		t.Errorf("bytes_freed should be omitted when zero, got %+v", decoded.Results[0].Extra)
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
	flushes := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "flush":
			flushes++
			b, _ := json.Marshal(map[string]any{"status": "flushed", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := flush.Run(mc, []string{"--namespace", "prod"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if flushes != 2 {
		t.Errorf("expected 2 flushes, got %d", flushes)
	}
}

func TestRun_NSWildcard_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
	}
	flushes := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "flush":
			flushes++
			b, _ := json.Marshal(map[string]any{"status": "flushed", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := flush.Run(mc, []string{"prod:*"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if flushes != 2 {
		t.Errorf("expected 2 flushes, got %d", flushes)
	}
}

func TestRun_NamespaceFlag_RejectsMixWithPositional(t *testing.T) {
	mc := &mockClient{response: map[string]any{}}
	err := flush.Run(mc, []string{"api", "--namespace", "prod"})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --namespace") {
		t.Errorf("err = %v", err)
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
	if err := flush.Run(mc, []string{"--namespace", "ghost"}); err == nil {
		t.Fatal("expected empty-namespace error")
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
