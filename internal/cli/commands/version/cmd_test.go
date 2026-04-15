package version_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/version"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	vinfo "github.com/Jaro-c/Lynx/internal/version"
)

type mockClient struct {
	response any
	err      error
}

func (m *mockClient) Call(_ string, _ any, result any) error {
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

func TestRun_NoDaemon(t *testing.T) {
	var buf bytes.Buffer
	err := version.Run(nil, &buf, []string{})
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Lynx CLI") {
		t.Error("Output missing 'Lynx CLI'")
	}
	if !strings.Contains(out, "Protocol") {
		t.Error("Output missing 'Protocol'")
	}
}

func TestRun_Help(t *testing.T) {
	var buf bytes.Buffer
	err := version.Run(nil, &buf, []string{"--help"})
	if err != nil {
		t.Fatalf("Run --help failed: %v", err)
	}
}

func TestRun_InvalidFlag(t *testing.T) {
	var buf bytes.Buffer
	err := version.Run(nil, &buf, []string{"--invalid"})
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
	if !strings.Contains(err.Error(), "Unknown flag") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_UnexpectedArgs(t *testing.T) {
	var buf bytes.Buffer
	err := version.Run(nil, &buf, []string{"arg1"})
	if err == nil {
		t.Fatal("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "Unexpected arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_DaemonSuccess(t *testing.T) {
	mc := &mockClient{
		response: vinfo.Info{
			Version:         "0.4.10",
			Commit:          "abc123",
			BuildDate:       "2026-04-14",
			ProtocolVersion: 1,
		},
	}
	var buf bytes.Buffer
	err := version.Run(mc, &buf, []string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Lynx Daemon") {
		t.Error("expected daemon section in output")
	}
}

func TestRun_ProtocolMismatch(t *testing.T) {
	mc := &mockClient{
		err: &protocol.RemoteError{
			Code:    "PROTOCOL_MISMATCH",
			Message: "incompatible",
			Data:    protocol.MismatchData{Supported: 2},
		},
	}
	var buf bytes.Buffer
	err := version.Run(mc, &buf, []string{})
	if err == nil {
		t.Fatal("expected error on protocol mismatch")
	}
	if !strings.Contains(err.Error(), "protocol mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_DaemonErrorOther(t *testing.T) {
	mc := &mockClient{err: errors.New("timeout")}
	var buf bytes.Buffer
	err := version.Run(mc, &buf, []string{})
	if err != nil {
		t.Errorf("expected no error on non-mismatch daemon error, got %v", err)
	}
	if !strings.Contains(buf.String(), "Protocol") {
		t.Error("expected Protocol section in output")
	}
}

func TestRun_JSON_NoDaemon(t *testing.T) {
	var buf bytes.Buffer
	err := version.Run(nil, &buf, []string{"--json"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	out := buf.String()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if _, ok := parsed["cli"]; !ok {
		t.Error("JSON missing 'cli' key")
	}
	if _, ok := parsed["protocol"]; !ok {
		t.Error("JSON missing 'protocol' key")
	}
}

func TestRun_JSON_WithDaemon(t *testing.T) {
	mc := &mockClient{
		response: vinfo.Info{
			Version:         "0.4.10",
			Commit:          "abc123",
			BuildDate:       "2026-04-14",
			ProtocolVersion: 1,
		},
	}
	var buf bytes.Buffer
	err := version.Run(mc, &buf, []string{"--json"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if _, ok := parsed["daemon"]; !ok {
		t.Error("JSON missing 'daemon' key with successful daemon call")
	}
}

func TestGetSpec(t *testing.T) {
	spec := version.GetSpec()
	if spec.Name != "version" {
		t.Errorf("expected name 'version', got %s", spec.Name)
	}
}
