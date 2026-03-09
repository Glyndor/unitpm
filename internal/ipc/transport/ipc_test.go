//go:build linux

package transport_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

func setupTestSocket(t *testing.T) {
	// Create a temp dir for socket
	dir, err := os.MkdirTemp("", "lynx-test-socket-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	
	// Set LYNX_SOCKET env var
	socketPath := strings.ReplaceAll(dir, "\\", "/") + "/lynx.sock"
	if err := os.Setenv("LYNX_SOCKET", socketPath); err != nil {
		t.Fatalf("failed to set LYNX_SOCKET: %v", err)
	}

	// Cleanup
	t.Cleanup(func() {
		_ = os.Unsetenv("LYNX_SOCKET")
		_ = os.RemoveAll(dir)
	})
}

func TestIPC(t *testing.T) {
	setupTestSocket(t)
	// Start server
	server := transport.NewServer()

	// Register ping handler
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
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
	setupTestSocket(t)
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
	setupTestSocket(t)
	server := transport.NewServer()
	server.Register("whoami", func(
		ctx context.Context,
		_ jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		id, ok := ctx.Value(transport.ContextKeyIdentity).(*transport.Identity)
		if !ok {
			return nil, errors.New("identity not found in context")
		}
		return jsonx.Marshal(id)
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
		t.Fatalf("Whoami failed: %v", err)
	}

	uid := os.Getuid()
	if identity.UID != strconv.Itoa(uid) {
		t.Errorf("Identity UID = %s, want %d", identity.UID, uid)
	}
}

func TestLimits(t *testing.T) {
	setupTestSocket(t)
	// Start server with small limits
	server := transport.NewServer()
	// Register echo handler
	server.Register("echo", func(
		_ context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
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
		if !strings.Contains(err.Error(), "ERR_LIMITS") &&
			!strings.Contains(err.Error(), "EOF") &&
			!strings.Contains(err.Error(), "connection reset") &&
			!strings.Contains(err.Error(), "timeout") &&
			!strings.Contains(err.Error(), "response ID mismatch") {
			// It's possible the server closes before sending response, or client fails to read response.
			// But valid behavior is error.
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestIPCAllowlistUIDs_Allowed(t *testing.T) {
	setupTestSocket(t)
	uid := os.Getuid()
	if err := os.Setenv("LYNX_IPC_ALLOW_UIDS", strconv.Itoa(uid)); err != nil {
		t.Fatalf("failed to set allowlist env: %v", err)
	}
	defer func() { _ = os.Unsetenv("LYNX_IPC_ALLOW_UIDS") }()

	server := transport.NewServer()
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
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

	var result map[string]string
	if err := client.Call("ping", nil, &result); err != nil {
		t.Fatalf("Ping with allowlist failed: %v", err)
	}
	if result["response"] != "pong" {
		t.Errorf("Unexpected response: got %v, want pong", result)
	}
}

func TestIPCAllowlistUIDs_Denied(t *testing.T) {
	setupTestSocket(t)
	if err := os.Setenv("LYNX_IPC_ALLOW_UIDS", "999999"); err != nil {
		t.Fatalf("failed to set allowlist env: %v", err)
	}
	defer func() { _ = os.Unsetenv("LYNX_IPC_ALLOW_UIDS") }()

	server := transport.NewServer()
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
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

	var result map[string]string
	err = client.Call("ping", nil, &result)
	if err == nil {
		t.Fatal("expected error for unauthorized client, got nil")
	}
	if !strings.Contains(err.Error(), "unauthorized") &&
		!strings.Contains(err.Error(), "permission") &&
		!strings.Contains(err.Error(), "denied") &&
		!strings.Contains(err.Error(), "pipe") &&
		!strings.Contains(err.Error(), "EOF") &&
		!strings.Contains(err.Error(), "reset") &&
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("unexpected error for unauthorized client: %v", err)
	}
}

func TestServerRecoverFromHandlerPanic(t *testing.T) {
	setupTestSocket(t)
	server := transport.NewServer()
	server.Register("panic", func(
		_ context.Context,
		_ jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		panic("boom")
	})
	server.Register("ping", func(
		_ context.Context,
		_ jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
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

	_ = client.Call("panic", nil, nil)

	client2, err := transport.NewClient()
	if err != nil {
		t.Fatalf("Failed to create second client: %v", err)
	}
	defer func() { _ = client2.Close() }()

	var result map[string]string
	if err := client2.Call("ping", nil, &result); err != nil {
		t.Fatalf("Ping after panic failed: %v", err)
	}
	if result["response"] != "pong" {
		t.Errorf("Unexpected response after panic: got %v, want pong", result)
	}
}
