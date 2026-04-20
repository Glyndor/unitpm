package reload_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
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
	if err == nil {
		t.Fatal("expected non-nil error so scripts see a non-zero exit")
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

func TestRun_JSONOutputAndPartialFailure(t *testing.T) {
	calls := 0
	mc := &scriptedMock{fn: func(cmd string, _ any, result any) error {
		calls++
		if calls == 2 {
			return errors.New("not found")
		}
		b, _ := json.Marshal(map[string]any{"status": "reloaded", "id": "x"})
		_ = json.Unmarshal(b, result)
		return nil
	}}
	got := captureStdout(t, func() {
		err := reload.Run(mc, []string{"a", "b", "c", "--json"})
		if err == nil {
			t.Error("expected non-nil error from 1 failed target")
		}
		if err != nil && !strings.Contains(err.Error(), "1 of 3") {
			t.Errorf("error should indicate 1 of 3 failed, got %v", err)
		}
	})
	var decoded struct {
		Op      string `json:"op"`
		Summary struct {
			Total, Ok, Failed int
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if decoded.Op != "reload" || decoded.Summary.Total != 3 ||
		decoded.Summary.Ok != 2 || decoded.Summary.Failed != 1 {
		t.Errorf("decoded = %+v", decoded)
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
	reloads := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "reload":
			reloads++
			b, _ := json.Marshal(map[string]any{"status": "reloaded", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := reload.Run(mc, []string{"--namespace", "prod"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if reloads != 2 {
		t.Errorf("expected 2 reloads, got %d", reloads)
	}
}

func TestRun_NSWildcard_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
	}
	reloads := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "reload":
			reloads++
			b, _ := json.Marshal(map[string]any{"status": "reloaded", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := reload.Run(mc, []string{"prod:*"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if reloads != 2 {
		t.Errorf("expected 2 reloads, got %d", reloads)
	}
}

func TestRun_NamespaceFlag_RejectsMixWithPositional(t *testing.T) {
	mc := &mockClient{response: map[string]any{}}
	err := reload.Run(mc, []string{"api", "--namespace", "prod"})
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
	if err := reload.Run(mc, []string{"--namespace", "ghost"}); err == nil {
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
