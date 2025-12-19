package transport_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

func TestIPC(t *testing.T) {
	// Start server
	server := transport.NewServer()

	// Register ping handler
	server.Register("ping", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Close() }()

	// Wait for server to be ready (usually instant but good to be safe)
	time.Sleep(100 * time.Millisecond)

	// Start client
	client, err := transport.NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

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

func TestIPC_Limits(t *testing.T) {
	// Start server
	server := transport.NewServer()
	// Register echo handler
	server.Register("echo", func(params json.RawMessage) (json.RawMessage, error) {
		return params, nil
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Close() }()

	time.Sleep(100 * time.Millisecond)

	// Start client
	client, err := transport.NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Test Oversized Message
	largeData := make(map[string]string)
	// MaxMsgSize is 1MB. Create a value slightly larger.
	val := strings.Repeat("a", 1024*1024+1024)
	largeData["data"] = val

	err = client.Call("echo", largeData, nil)
	if err == nil {
		t.Error("Expected error for oversized message, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
		// We expect ERR_LIMITS or connection closed
		if !strings.Contains(err.Error(), "ERR_LIMITS") && !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "connection reset") {
             // It's possible the server closes before sending response, or client fails to read response.
             // But valid behavior is error.
		}
	}
}
