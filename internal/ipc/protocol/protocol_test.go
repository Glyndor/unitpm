//go:build linux

package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestVersion_Constant(t *testing.T) {
	if protocol.Version != 1 {
		t.Errorf("Version = %d, want 1", protocol.Version)
	}
}

func TestStatusConstants(t *testing.T) {
	if protocol.StatusError != "error" {
		t.Errorf("StatusError = %q, want error", protocol.StatusError)
	}
	if protocol.StatusSuccess != "success" {
		t.Errorf("StatusSuccess = %q, want success", protocol.StatusSuccess)
	}
}

func TestRequest_JSONRoundTrip(t *testing.T) {
	params, _ := json.Marshal(map[string]string{"id": "abc"})
	req := protocol.Request{
		Version: 1,
		ID:      "req-123",
		Command: "stop",
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.Request
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != 1 || got.ID != "req-123" || got.Command != "stop" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestResponse_ErrorCase(t *testing.T) {
	resp := protocol.Response{
		Version: 1,
		ID:      "req-1",
		Status:  protocol.StatusError,
		Error:   &protocol.Error{Code: "ERR_NOT_FOUND", Message: "process not found"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != protocol.StatusError {
		t.Errorf("status = %q, want error", got.Status)
	}
	if got.Error == nil {
		t.Fatal("error field should not be nil")
	}
	if got.Error.Code != "ERR_NOT_FOUND" {
		t.Errorf("error code = %q, want ERR_NOT_FOUND", got.Error.Code)
	}
}

func TestResponse_SuccessCase(t *testing.T) {
	result, _ := json.Marshal(map[string]string{"id": "abc123"})
	resp := protocol.Response{
		Version: 1,
		ID:      "req-2",
		Status:  protocol.StatusSuccess,
		Result:  result,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != protocol.StatusSuccess {
		t.Errorf("status = %q, want success", got.Status)
	}
	if got.Error != nil {
		t.Errorf("error field should be nil on success, got %+v", got.Error)
	}
}

func TestAppSpec_JSONRoundTrip(t *testing.T) {
	spec := protocol.AppSpec{
		Version:   1,
		ID:        "uuid-001",
		Name:      "myapp",
		Namespace: "production",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "node",
			Args:    []string{"server.js", "--port", "8080"},
			Shell:   false,
		},
		Cwd:     "/app",
		Env:     map[string]string{"PORT": "8080"},
		EnvFile: ".env",
		Logs: &protocol.AppLogs{
			Mode:      "file",
			Dir:       "/var/log/myapp",
			Stdout:    "out.log",
			Stderr:    "err.log",
			Format:    "json",
			Timestamp: "rfc3339",
		},
		Restart: &protocol.AppRestart{
			Policy:      "on-failure",
			MaxRetries:  3,
			BackoffMs:   500,
			BackoffType: "expo",
			StopOnExit:  []int{0, 2},
		},
		RunAs: &protocol.RunAsPolicy{Mode: "self"},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.AppSpec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Name != spec.Name {
		t.Errorf("name = %q, want %q", got.Name, spec.Name)
	}
	if got.Exec.Command != "node" {
		t.Errorf("exec.command = %q, want node", got.Exec.Command)
	}
	if len(got.Exec.Args) != 3 {
		t.Errorf("exec.args len = %d, want 3", len(got.Exec.Args))
	}
	if got.Logs == nil || got.Logs.Mode != "file" {
		t.Errorf("logs.mode mismatch")
	}
	if got.Restart == nil || got.Restart.Policy != "on-failure" {
		t.Errorf("restart.policy mismatch")
	}
	if len(got.Restart.StopOnExit) != 2 {
		t.Errorf("stop_on_exit len = %d, want 2", len(got.Restart.StopOnExit))
	}
	if got.RunAs == nil || got.RunAs.Mode != "self" {
		t.Errorf("runAs.mode mismatch")
	}
}

func TestAppSpec_OmitEmptyFields(t *testing.T) {
	spec := protocol.AppSpec{
		Version: 1,
		Name:    "minimal",
		Exec:    protocol.AppExec{Type: "command", Command: "echo"},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Fields with omitempty should not appear when zero
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := raw["logs"]; ok {
		t.Error("logs should be omitted when nil")
	}
	if _, ok := raw["restart"]; ok {
		t.Error("restart should be omitted when nil")
	}
	if _, ok := raw["runAs"]; ok {
		t.Error("runAs should be omitted when nil")
	}
	if _, ok := raw["namespace"]; ok {
		t.Error("namespace should be omitted when empty")
	}
}

func TestStartRequest_JSONRoundTrip(t *testing.T) {
	req := protocol.StartRequest{
		ProtocolVersion: 1,
		RequestID:       "start-001",
		Type:            "start",
		Spec: protocol.AppSpec{
			Version: 1,
			Name:    "api",
			Exec:    protocol.AppExec{Type: "command", Command: "go"},
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.StartRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "start" {
		t.Errorf("type = %q, want start", got.Type)
	}
	if got.Spec.Name != "api" {
		t.Errorf("spec.name = %q, want api", got.Spec.Name)
	}
}

func TestStartResponse_OkCase(t *testing.T) {
	resp := protocol.StartResponse{
		ProtocolVersion: 1,
		Type:            "start",
		RequestID:       "start-001",
		Ok:              true,
		Data: &protocol.StartResponseData{
			ID:     "proc-123",
			PID:    42,
			Status: "running",
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.StartResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Ok {
		t.Error("expected Ok=true")
	}
	if got.Data == nil {
		t.Fatal("data should not be nil")
	}
	if got.Data.PID != 42 {
		t.Errorf("pid = %d, want 42", got.Data.PID)
	}
}

func TestRemoteError_Error(t *testing.T) {
	re := &protocol.RemoteError{
		Code:    "ERR_TEST",
		Message: "something went wrong",
	}
	got := re.Error()
	want := "ipc error: [ERR_TEST] something went wrong"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestRemoteError_ImplementsError(t *testing.T) {
	// Compile-time check: *RemoteError satisfies the error interface.
	var _ error = (*protocol.RemoteError)(nil)
	_ = t
}

func TestStartResponse_ErrorCase(t *testing.T) {
	resp := protocol.StartResponse{
		ProtocolVersion: 1,
		Type:            "start",
		RequestID:       "start-002",
		Ok:              false,
		Error:           &protocol.StartError{Code: "ERR_LIMIT", Message: "too many processes"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.StartResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Ok {
		t.Error("expected Ok=false")
	}
	if got.Error == nil {
		t.Fatal("error should not be nil")
	}
	if got.Error.Code != "ERR_LIMIT" {
		t.Errorf("error code = %q, want ERR_LIMIT", got.Error.Code)
	}
}
