package protocol

import (
	"encoding/json"
	"testing"

	"github.com/Jaro-c/Lynx/internal/jsonx"
)

func TestRequestMarshalling(t *testing.T) {
	req := Request{
		Version: 1,
		ID:      "123",
		Command: "ping",
		Params:  jsonx.RawMessage(`{"foo":"bar"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != req.ID {
		t.Errorf("Expected ID %s, got %s", req.ID, decoded.ID)
	}
	if decoded.Command != req.Command {
		t.Errorf("Expected Command %s, got %s", req.Command, decoded.Command)
	}
}

func TestStartResponseMarshalling(t *testing.T) {
	resp := StartResponse{
		ProtocolVersion: 1,
		Type:            "start",
		RequestID:       "req-1",
		Ok:              true,
		Data: &StartResponseData{
			ID:      "app-1",
			PID:     100,
			Message: "started",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded StartResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !decoded.Ok {
		t.Error("Expected Ok=true")
	}
	if decoded.Data == nil {
		t.Fatal("Expected Data not nil")
	}
	if decoded.Data.PID != 100 {
		t.Errorf("Expected PID 100, got %d", decoded.Data.PID)
	}
}

func TestAppSpecMarshalling(t *testing.T) {
	spec := AppSpec{
		Version: 1,
		ID:      "test-app",
		Name:    "Test App",
		Exec: AppExec{
			Type:    "command",
			Command: "echo hello",
		},
		Restart: &AppRestart{
			Policy:     "always",
			MaxRetries: 3,
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded AppSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Name != spec.Name {
		t.Errorf("Expected Name %s, got %s", spec.Name, decoded.Name)
	}
	if decoded.Restart == nil {
		t.Fatal("Expected Restart not nil")
	}
	if decoded.Restart.Policy != "always" {
		t.Errorf("Expected Policy always, got %s", decoded.Restart.Policy)
	}
}
