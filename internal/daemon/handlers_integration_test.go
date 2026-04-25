//go:build linux

package daemon_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/daemon"
	"github.com/Jaro-c/Lynx/internal/daemon/audit"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/version"
)

// setupE2E wires a full daemon->IPC->client stack against a throwaway
// Unix socket and returns a connected client plus the manager. All
// resources are torn down on t.Cleanup.
func setupE2E(t *testing.T) (transport.IPCClient, *manager.Manager) {
	t.Helper()

	// Temp XDG dirs for spec/log storage.
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "lynx", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", tempDir)
	t.Setenv("HOME", tempDir)

	// Explicit socket path inside temp dir.
	socketPath := filepath.Join(tempDir, "lynx.sock")
	t.Setenv("LYNX_SOCKET", socketPath)

	mgr := manager.NewManager()
	server := transport.NewServer()
	daemon.RegisterHandlers(server, mgr, false /*privileged*/, audit.Disabled())

	if err := server.Start(); err != nil {
		t.Fatalf("server.Start: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	time.Sleep(100 * time.Millisecond) // server ready

	client, err := transport.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client, mgr
}

func TestE2E_Ping(t *testing.T) {
	client, _ := setupE2E(t)

	var resp map[string]string
	if err := client.Call("ping", nil, &resp); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if resp["response"] != "pong" {
		t.Errorf("got %v, want pong", resp)
	}
}

func TestE2E_Version(t *testing.T) {
	client, _ := setupE2E(t)

	var got version.Info
	if err := client.Call("version", nil, &got); err != nil {
		t.Fatalf("version: %v", err)
	}
	if got.Version == "" {
		t.Error("version Info returned empty Version field")
	}
}

func TestE2E_List_Empty(t *testing.T) {
	client, _ := setupE2E(t)

	var list []map[string]any
	if err := client.Call("list", nil, &list); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d entries", len(list))
	}
}

func TestE2E_StartThenList(t *testing.T) {
	client, mgr := setupE2E(t)

	// Seed a process directly via the manager, bypassing the start handler
	// (which requires extra spec validation scaffolding we already unit-test).
	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-list", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}
	defer func() { _ = mgr.Stop(id) }()

	// List should now report 1 entry.
	var list []map[string]any
	if err := client.Call("list", nil, &list); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry after seed, got %d", len(list))
	}
	if list[0]["name"] != "e2e-list" {
		t.Errorf("unexpected name in list: %v", list[0]["name"])
	}
}

func TestE2E_Show_ByID(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-show", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}
	defer func() { _ = mgr.Stop(id) }()

	var resp map[string]any
	if err := client.Call("show", map[string]string{"id": id}, &resp); err != nil {
		t.Fatalf("show: %v", err)
	}
	if resp["info"] == nil {
		t.Error("expected info field in show response")
	}
	if resp["spec"] == nil {
		t.Error("expected spec field in show response")
	}
}

func TestE2E_Show_NotFound(t *testing.T) {
	client, _ := setupE2E(t)

	var resp map[string]any
	err := client.Call("show", map[string]string{"id": "does-not-exist"}, &resp)
	if err == nil {
		t.Fatal("expected error for show of unknown ID")
	}
}

func TestE2E_Stop_Roundtrip(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-stop", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}

	var resp map[string]any
	if err := client.Call("stop", map[string]string{"id": id}, &resp); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if resp["status"] != "stopped" {
		t.Errorf("unexpected status: %v", resp["status"])
	}
	if resp["id"] != id {
		t.Errorf("unexpected id: %v", resp["id"])
	}
}

func TestE2E_Delete_Roundtrip(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-del", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}

	var resp map[string]any
	if err := client.Call("delete", map[string]any{"id": id, "purge": false}, &resp); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp["status"] != "deleted" {
		t.Errorf("unexpected status: %v", resp["status"])
	}

	// Manager should no longer know about this id.
	if _, ok := mgr.Get(id); ok {
		t.Error("process still in manager after delete")
	}
}

func TestE2E_Flush_BytesFreed(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := filepath.Join(t.TempDir(), "logs", id)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	stdoutPath := filepath.Join(logDir, "stdout.log")
	stderrPath := filepath.Join(logDir, "stderr.log")
	// Seed content in both files so the handler has bytes to reclaim.
	if err := os.WriteFile(stdoutPath, []byte("hello stdout\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("hello stderr\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-flush", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: stdoutPath,
			Stderr: stderrPath,
		},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}
	defer func() { _ = mgr.Stop(id) }()

	// Read actual on-disk sizes after Start (which appends a STARTED banner)
	// so the assertion is robust to banner length changes.
	siOut, err := os.Stat(stdoutPath)
	if err != nil {
		t.Fatalf("stat stdout pre-flush: %v", err)
	}
	siErr, err := os.Stat(stderrPath)
	if err != nil {
		t.Fatalf("stat stderr pre-flush: %v", err)
	}
	before := siOut.Size() + siErr.Size()

	var resp map[string]any
	if err := client.Call("flush", map[string]string{"id": id}, &resp); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if resp["status"] != "flushed" {
		t.Errorf("status = %v, want flushed", resp["status"])
	}
	// JSON numbers decode into float64 through map[string]any.
	got, ok := resp["bytes_freed"].(float64)
	if !ok {
		t.Fatalf("bytes_freed missing or wrong type: %T %v", resp["bytes_freed"], resp["bytes_freed"])
	}
	if int64(got) != before {
		t.Errorf("bytes_freed = %d, want %d", int64(got), before)
	}

	// Files should be truncated on disk.
	for _, p := range []string{stdoutPath, stderrPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("stat %s: %v", p, err)
			continue
		}
		if info.Size() != 0 {
			t.Errorf("expected %s truncated, size=%d", p, info.Size())
		}
	}
}

func TestE2E_Scale_NoTemplate(t *testing.T) {
	client, _ := setupE2E(t)

	var resp any
	err := client.Call("scale",
		map[string]any{"namespace": "default", "name": "ghost", "target": 3}, &resp)
	if err == nil {
		t.Fatal("expected error scaling nonexistent template")
	}
}

func TestE2E_Reset_ByID(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-reset", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}
	defer func() { _ = mgr.Stop(id) }()

	var resp map[string]any
	if err := client.Call("reset", map[string]string{"id": id}, &resp); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if resp["status"] != "reset" {
		t.Errorf("unexpected status: %v", resp["status"])
	}
}

func TestE2E_Restart_ByID(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-restart", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}
	defer func() { _ = mgr.Stop(id) }()

	var resp map[string]any
	if err := client.Call("restart", map[string]string{"id": id}, &resp); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if resp["status"] != "restarted" {
		t.Errorf("unexpected status: %v", resp["status"])
	}
}

func TestE2E_ResolveByName(t *testing.T) {
	client, mgr := setupE2E(t)

	id := uuid.Must(uuid.NewV7()).String()
	s := protocol.AppSpec{
		Version: 1, ID: id, Name: "e2e-resolve", Namespace: "default",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec: %v", err)
	}
	defer func() { _ = mgr.Stop(id) }()

	// Issue stop by name — handler must resolve it to the real id.
	var resp map[string]any
	if err := client.Call("stop", map[string]string{"id": "e2e-resolve"}, &resp); err != nil {
		t.Fatalf("stop by name: %v", err)
	}
	if resp["id"] != id {
		t.Errorf("handler did not resolve name to id: got %v want %s", resp["id"], id)
	}
}
