package ipc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc"
)

func TestIPC(t *testing.T) {
	// Start server
	server := ipc.NewServer()
	
	// Register ping handler
	server.Register("ping", func(params json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// Wait for server to be ready (usually instant but good to be safe)
	time.Sleep(100 * time.Millisecond)

	// Start client
	client, err := ipc.NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test Ping
	var result map[string]string
	if err := client.Call("ping", nil, &result); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if result["response"] != "pong" {
		t.Errorf("Unexpected response: got %v, want pong", result)
	}

	// Test Unknown Command
	err = client.Call("unknown", nil, nil)
	if err == nil {
		t.Error("Expected error for unknown command, got nil")
	} else {
		// We expect an IPC error with code UNKNOWN_COMMAND
		// The error string format is "ipc error: [CODE] Message"
		expected := "ipc error: [UNKNOWN_COMMAND] Command not found"
		if err.Error() != expected {
			t.Errorf("Unexpected error message: got %q, want %q", err.Error(), expected)
		}
	}
}
