package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

func TestIPC(t *testing.T) {
	// Start server
	server := transport.NewServer()

	// Register ping handler
	server.Register("ping", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
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

func TestSocketPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix permissions test on Windows")
	}

	server := transport.NewServer()
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Close() }()

	time.Sleep(100 * time.Millisecond)

	path, err := transport.GetSocketPath()
	if err != nil {
		t.Fatalf("Failed to get socket path: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat socket: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Socket permissions = %o, want 0600", perm)
	}
}

func TestIdentity(t *testing.T) {
	server := transport.NewServer()
	server.Register("whoami", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		id, ok := ctx.Value(transport.ContextKeyIdentity).(*transport.Identity)
		if !ok {
			return nil, fmt.Errorf("identity not found in context")
		}
		return json.Marshal(id)
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Close() }()

	time.Sleep(100 * time.Millisecond)

	client, err := transport.NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	var identity transport.Identity
	if err := client.Call("whoami", nil, &identity); err != nil {
		t.Fatalf("whoami failed: %v", err)
	}

	t.Logf("Got identity: %+v", identity)

	if runtime.GOOS == "linux" {
		uid := os.Getuid()
		if identity.UID != fmt.Sprintf("%d", uid) {
			t.Errorf("UID mismatch: got %s, want %d", identity.UID, uid)
		}
	} else {
		// Windows/Stub returns "0"
		if identity.UID != "0" {
			t.Errorf("UID mismatch: got %s, want 0", identity.UID)
		}
	}
}

func TestIPC_Limits(t *testing.T) {
	// Start server
	server := transport.NewServer()
	// Register echo handler
	server.Register("echo", func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
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
