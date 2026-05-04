//go:build !windows

package transport_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

func TestGetSocketPath_AbsoluteEnvOverride(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	t.Setenv("LYNX_SOCKET", sockPath)

	got, err := transport.GetSocketPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sockPath {
		t.Errorf("got %q, want %q", got, sockPath)
	}
}

func TestGetSocketPath_RelativePathRejected(t *testing.T) {
	t.Setenv("LYNX_SOCKET", "relative/path/lynx.sock")

	_, err := transport.GetSocketPath()
	if err == nil {
		t.Fatal("expected error for relative LYNX_SOCKET path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error should mention absolute path, got: %v", err)
	}
}

func TestGetSocketPath_WorldWritableParentRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv("LYNX_SOCKET", filepath.Join(dir, "lynx.sock"))

	_, err := transport.GetSocketPath()
	if err == nil {
		t.Fatal("expected error for world-writable parent dir, got nil")
	}
	if !strings.Contains(err.Error(), "world-writable") {
		t.Errorf("error should mention world-writable, got: %v", err)
	}
}

func TestGetSocketPath_MissingXDGRuntimeDir(t *testing.T) {
	// Only meaningful for non-root non-lynx users.
	if os.Getuid() == 0 {
		t.Skip("running as root; XDG_RUNTIME_DIR check is bypassed")
	}

	t.Setenv("LYNX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	_, err := transport.GetSocketPath()
	if err == nil {
		t.Fatal("expected error when XDG_RUNTIME_DIR is unset, got nil")
	}
	if !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Errorf("error should mention XDG_RUNTIME_DIR, got: %v", err)
	}
}

func TestGetSocketPath_XDGRuntimeDirUsed(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; uses fixed /run/lynxd/lynx.sock instead")
	}

	dir := t.TempDir()
	t.Setenv("LYNX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", dir)

	got, err := transport.GetSocketPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Errorf("socket path %q should be under XDG_RUNTIME_DIR %q", got, dir)
	}
	if !strings.HasSuffix(got, "lynx.sock") {
		t.Errorf("socket path %q should end with lynx.sock", got)
	}
}

func TestGetSocketPath_EnvOverridePrecedesXDG(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit.sock")
	xdgDir := t.TempDir()

	t.Setenv("LYNX_SOCKET", explicit)
	t.Setenv("XDG_RUNTIME_DIR", xdgDir)

	got, err := transport.GetSocketPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != explicit {
		t.Errorf("LYNX_SOCKET override ignored: got %q, want %q", got, explicit)
	}
}

func TestDaemonUnreachableError_ConnectionRefused(t *testing.T) {
	// Create a real but unused socket path to trigger "connection refused".
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "nope.sock")
	t.Setenv("LYNX_SOCKET", sockPath)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	// NewClient will fail because nothing listens at sockPath.
	_, err := transport.NewClient()
	if err == nil {
		t.Fatal("expected error when daemon not running, got nil")
	}
	msg := err.Error()
	// Error should guide user toward starting the daemon.
	if !strings.Contains(msg, "cannot reach") && !strings.Contains(msg, "lynxd") {
		t.Errorf("error message not user-friendly: %v", err)
	}
}

func TestDaemonUnreachableError_UserModeHint(t *testing.T) {
	dir := t.TempDir()
	// Simulate XDG_RUNTIME_DIR path so daemonUnreachable detects user mode.
	sockPath := filepath.Join(dir, "run", "user", "1000", "lynx.sock")
	if err := os.MkdirAll(filepath.Dir(sockPath), 0700); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	t.Setenv("LYNX_SOCKET", sockPath)
	t.Setenv("XDG_RUNTIME_DIR", dir+"/run/user/1000")

	_, err := transport.NewClient()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "lynxd") {
		t.Errorf("user-mode error should mention lynxd: %v", err)
	}
}
