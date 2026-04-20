package deletecmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
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
	// Per-target errors are printed as they happen but Run returns a
	// non-nil error so scripts see a non-zero exit code.
	err := deletecmd.Run(mc, []string{"abc-123"})
	if err == nil {
		t.Fatal("expected non-nil error")
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

func TestRun_JSONOutput(t *testing.T) {
	calls := 0
	mc := &scriptedMock{fn: func(cmd string, _ any, result any) error {
		calls++
		if calls == 2 {
			return errors.New("not found")
		}
		b, _ := json.Marshal(map[string]any{"status": "deleted", "id": "x"})
		_ = json.Unmarshal(b, result)
		return nil
	}}
	got := captureStdout(t, func() {
		err := deletecmd.Run(mc, []string{"a", "b", "c", "--json"})
		if err == nil {
			t.Error("expected non-nil error from 1 failed target")
		}
	})
	var decoded batchShape
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, got)
	}
	if decoded.Op != "delete" {
		t.Errorf("op=%q", decoded.Op)
	}
	if decoded.Summary.Total != 3 || decoded.Summary.Ok != 2 || decoded.Summary.Failed != 1 {
		t.Errorf("summary = %+v", decoded.Summary)
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("expected pure JSON, got:\n%s", got)
	}
}

func TestRun_FlagsAnywhere(t *testing.T) {
	// Ensure --json and --purge work regardless of position.
	mc := &mockClient{response: map[string]any{"status": "deleted", "id": "x"}}
	_ = captureStdout(t, func() {
		if err := deletecmd.Run(mc, []string{"--json", "a"}); err != nil {
			t.Errorf("flags-first: %v", err)
		}
	})
	mc2 := &mockClient{response: map[string]any{"status": "deleted", "id": "x"}}
	_ = captureStdout(t, func() {
		if err := deletecmd.Run(mc2, []string{"a", "--json"}); err != nil {
			t.Errorf("flags-last: %v", err)
		}
	})
	mc3 := &mockClient{response: map[string]any{"status": "deleted", "id": "x"}}
	if err := deletecmd.Run(mc3, []string{"a", "--purge", "b"}); err != nil {
		t.Errorf("flags-middle: %v", err)
	}
	if len(mc3.params) != 2 {
		t.Fatalf("expected 2 calls for 2 positional IDs, got %d", len(mc3.params))
	}
	b, _ := json.Marshal(mc3.params[0])
	if !strings.Contains(string(b), `"purge":true`) {
		t.Errorf("--purge not applied when interspersed: %s", b)
	}
}

type batchShape struct {
	Op      string `json:"op"`
	Results []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	} `json:"results"`
	Summary struct {
		Total  int `json:"total"`
		Ok     int `json:"ok"`
		Failed int `json:"failed"`
		Noop   int `json:"noop"`
	} `json:"summary"`
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
	deletes := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "delete":
			deletes++
			b, _ := json.Marshal(map[string]any{"status": "deleted", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := deletecmd.Run(mc, []string{"--namespace", "prod"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if deletes != 2 {
		t.Errorf("expected 2 deletes, got %d", deletes)
	}
}

func TestRun_NSWildcard_ExpandsAllProcsInNS(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
		{"id": "id-prod-worker", "name": "worker", "namespace": "prod"},
	}
	deletes := 0
	mc := &scriptedMock{fn: func(cmd string, _, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "delete":
			deletes++
			b, _ := json.Marshal(map[string]any{"status": "deleted", "id": "x"})
			_ = json.Unmarshal(b, result)
		}
		return nil
	}}
	if err := deletecmd.Run(mc, []string{"prod:*"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if deletes != 2 {
		t.Errorf("expected 2 deletes, got %d", deletes)
	}
}

func TestRun_NamespaceFlag_PreservesPurge(t *testing.T) {
	procs := []map[string]any{
		{"id": "id-prod-api", "name": "api", "namespace": "prod"},
	}
	var sawPurge bool
	mc := &scriptedMock{fn: func(cmd string, params, result any) error {
		switch cmd {
		case "list":
			b, _ := json.Marshal(procs)
			_ = json.Unmarshal(b, result)
		case "delete":
			b, _ := json.Marshal(params)
			if strings.Contains(string(b), `"purge":true`) {
				sawPurge = true
			}
			b2, _ := json.Marshal(map[string]any{"status": "deleted", "id": "x"})
			_ = json.Unmarshal(b2, result)
		}
		return nil
	}}
	if err := deletecmd.Run(mc, []string{"--namespace", "prod", "--purge"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !sawPurge {
		t.Error("--purge must be applied to namespace-expanded targets")
	}
}

func TestRun_NamespaceFlag_RejectsMixWithPositional(t *testing.T) {
	mc := &mockClient{response: map[string]any{}}
	err := deletecmd.Run(mc, []string{"api", "--namespace", "prod"})
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
	if err := deletecmd.Run(mc, []string{"--namespace", "ghost"}); err == nil {
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
