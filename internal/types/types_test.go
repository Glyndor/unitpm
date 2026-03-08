package types

import (
	"encoding/json"
	"testing"
)

func TestProcessInfoMarshalling(t *testing.T) {
	info := ProcessInfo{
		ID:    "test-1",
		Name:  "test-process",
		State: StateRunning,
		PID:   1234,
		CPU:   10.5,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ProcessInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != info.ID {
		t.Errorf("Expected ID %s, got %s", info.ID, decoded.ID)
	}
	if decoded.State != StateRunning {
		t.Errorf("Expected State running, got %s", decoded.State)
	}
	if decoded.CPU != 10.5 {
		t.Errorf("Expected CPU 10.5, got %f", decoded.CPU)
	}
}

func TestProcessStateConstants(t *testing.T) {
	if StateRunning != "running" {
		t.Error("StateRunning constant incorrect")
	}
	if StateOnline != "online" {
		t.Error("StateOnline constant incorrect")
	}
	if StateStopped != "stopped" {
		t.Error("StateStopped constant incorrect")
	}
}
