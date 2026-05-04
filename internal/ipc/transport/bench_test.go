//go:build linux

package transport_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

// setupBenchSocket is like setupTestSocket but uses testing.B.
func setupBenchSocket(b *testing.B) {
	b.Helper()
	dir, err := os.MkdirTemp("", "lynx-bench-socket-*")
	if err != nil {
		b.Fatalf("mkdirtemp: %v", err)
	}
	sockPath := strings.ReplaceAll(dir, "\\", "/") + "/lynx.sock"
	if err := os.Setenv("LYNX_SOCKET", sockPath); err != nil {
		b.Fatalf("setenv: %v", err)
	}
	b.Cleanup(func() {
		_ = os.Unsetenv("LYNX_SOCKET")
		_ = os.RemoveAll(dir)
	})
}

// disableRateLimit sets the token-bucket limits high enough that benchmarks
// never hit them. Must be called before transport.NewServer().
func disableRateLimit(b *testing.B) {
	b.Helper()
	b.Setenv("LYNX_IPC_RATE_BURST", "10000000")
	b.Setenv("LYNX_IPC_RATE_PER_SEC", "10000000")
}

// BenchmarkIPCRoundTrip measures the full latency of one client.Call through
// the Unix-socket transport: marshal → write → read → unmarshal. This is the
// hot path hit on every lynxpm command.
func BenchmarkIPCRoundTrip(b *testing.B) {
	setupBenchSocket(b)
	disableRateLimit(b)

	server := transport.NewServer()
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
	})
	if err := server.Start(); err != nil {
		b.Fatalf("server start: %v", err)
	}
	b.Cleanup(func() { _ = server.Close() })
	time.Sleep(50 * time.Millisecond)

	client, err := transport.NewClient()
	if err != nil {
		b.Fatalf("client: %v", err)
	}
	b.Cleanup(func() { _ = client.Close() })

	var result map[string]string
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Call("ping", nil, &result); err != nil {
			b.Fatalf("call: %v", err)
		}
	}
}

// BenchmarkIPCRoundTrip_WithPayload measures round-trip with a realistic
// payload size (~1 KB params + response) to surface serialization overhead.
func BenchmarkIPCRoundTrip_WithPayload(b *testing.B) {
	setupBenchSocket(b)
	disableRateLimit(b)

	server := transport.NewServer()
	server.Register("echo", func(_ context.Context, p jsonx.RawMessage) (jsonx.RawMessage, error) {
		return p, nil
	})
	if err := server.Start(); err != nil {
		b.Fatalf("server start: %v", err)
	}
	b.Cleanup(func() { _ = server.Close() })
	time.Sleep(50 * time.Millisecond)

	client, err := transport.NewClient()
	if err != nil {
		b.Fatalf("client: %v", err)
	}
	b.Cleanup(func() { _ = client.Close() })

	payload := map[string]string{
		"name":      "my-api-service",
		"namespace": "production",
		"command":   "node",
		"cwd":       "/var/www/app",
		"entry":     "dist/index.js",
		"env_var_1": strings.Repeat("x", 64),
		"env_var_2": strings.Repeat("y", 64),
		"env_var_3": strings.Repeat("z", 64),
	}

	var result map[string]string
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Call("echo", payload, &result); err != nil {
			b.Fatalf("call: %v", err)
		}
	}
}

// BenchmarkGetSocketPath measures the path-resolution logic called on every
// client connect. It exercises the LYNX_SOCKET fast path.
func BenchmarkGetSocketPath_EnvOverride(b *testing.B) {
	dir := b.TempDir()
	if err := os.Setenv("LYNX_SOCKET", filepath.Join(dir, "lynx.sock")); err != nil {
		b.Fatalf("setenv: %v", err)
	}
	b.Cleanup(func() { _ = os.Unsetenv("LYNX_SOCKET") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := transport.GetSocketPath(); err != nil {
			b.Fatalf("GetSocketPath: %v", err)
		}
	}
}
