//go:build linux

package manager_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/spec"
)

func TestRestoreAndPersistence(t *testing.T) {
	// Setup environment
	tempDir, err := os.MkdirTemp("", "lynx-mgr-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Ensure log directory exists to avoid process start failure
	logDir := filepath.Join(tempDir, "lynx/logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", tempDir)

	// 1. Create specs on disk
	// App A: Enabled, should start
	idA := uuid.Must(uuid.NewV7()).String()
	specA := protocol.AppSpec{
		Version: 1, ID: idA, Name: "app-a",
		Exec: protocol.AppExec{Type: "command", Command: "true"}, // Short lived is fine for this test
	}
	if _, err := spec.SaveSpec(idA, specA); err != nil {
		t.Fatal(err)
	}

	// App B: Disabled, should NOT start
	idB := uuid.Must(uuid.NewV7()).String()
	specB := protocol.AppSpec{
		Version: 1, ID: idB, Name: "app-b",
		Exec:     protocol.AppExec{Type: "command", Command: "true"},
		Disabled: true,
	}
	if _, err := spec.SaveSpec(idB, specB); err != nil {
		t.Fatal(err)
	}

	// 2. Initialize Manager and Restore
	mgr := manager.NewManager()
	if err := mgr.Restore(); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}

	// 3. Verify Restore Behavior
	// App A should exist in manager
	if _, exists := mgr.Get(idA); !exists {
		t.Error("App A (enabled) was not restored")
	}

	// App B should NOT exist in manager
	if _, exists := mgr.Get(idB); exists {
		t.Error("App B (disabled) was incorrectly restored")
	}

	// 4. Test Stop Persistence
	// Stop App A -> Should update spec to Disabled=true
	// Note: App A might have exited already because it's just "true", but Stop() handles cleanup.
	// For testing persistence, we need to ensure Stop() is called even if exited.
	// However, Stop() checks if running. If "true" finished, State is Exited.
	// We need a long running process to test Stop() effectively.

	// Let's create App C: Long running
	idC := uuid.Must(uuid.NewV7()).String()
	specC := protocol.AppSpec{
		Version: 1, ID: idC, Name: "app-c",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"10"}},
	}

	// Start App C manually (StartWithSpec)
	if _, err := mgr.StartWithSpec(specC); err != nil {
		t.Fatalf("StartWithSpec(C) failed: %v", err)
	}

	// Wait a bit to ensure it's running
	time.Sleep(100 * time.Millisecond)

	// Stop App C
	if err := mgr.Stop(idC); err != nil {
		t.Fatalf("Stop(C) failed: %v", err)
	}

	// Check on-disk spec for App C
	loadedSpecs, _ := spec.LoadAll()
	var loadedC protocol.AppSpec
	foundC := false
	for _, s := range loadedSpecs {
		if s.ID == idC {
			loadedC = s
			foundC = true
			break
		}
	}

	if !foundC {
		t.Fatal("Spec C not found on disk after Stop")
	}
	if !loadedC.Disabled {
		t.Error("Spec C was not marked Disabled=true after Stop()")
	}

	// 5. Test Start Persistence (Resurrection Re-enabling)
	// Start App C again -> Should set Disabled=false

	// Note: We need to use the spec from disk or original spec, but ensure Disabled is passed if we want to test clearing?
	// Actually StartWithSpec takes an AppSpec. The CLI usually loads the spec or creates a new one.
	// If we pass the *disabled* spec to StartWithSpec, it should clear the flag.
	loadedC.Disabled = true // Ensure input has it true

	// Clean up process entry from manager first if needed?
	// Manager.Stop doesn't remove it from map, just stops it.
	// StartWithSpec checks if exists. We need to Delete it first or Restart it?
	// Existing Manager.StartWithSpec returns error if ID exists.
	// So we must Delete(id) before starting again, OR use Restart(id).
	// But the requirement says "Starting an app manually...".
	// If we use CLI "start", it generates a new ID usually unless we restart.
	// If we use "restart", that calls manager.Restart().
	// Let's check manager.Delete().

	if err := mgr.Delete(idC); err != nil {
		t.Fatalf("Delete(C) failed: %v", err)
	}

	// Now start again with the same ID (simulating user re-running start command with same config, or just checking logic)
	// Passing the spec which currently has Disabled=true (from our load)
	if _, err := mgr.StartWithSpec(loadedC); err != nil {
		t.Fatalf("StartWithSpec(C) restart failed: %v", err)
	}

	// Check disk again
	loadedSpecs, _ = spec.LoadAll()
	for _, s := range loadedSpecs {
		if s.ID == idC {
			if s.Disabled {
				t.Error("Spec C was not marked Disabled=false after StartWithSpec()")
			}
			break
		}
	}

	// Cleanup process C
	_ = mgr.Stop(idC)
}

func TestManager_MaxProcessesLimit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lynx-mgr-maxproc")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	t.Setenv("XDG_CONFIG_HOME", tempDir)

	logDir := filepath.Join(tempDir, "lynx/logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", tempDir)

	if err := os.Setenv("LYNX_MAX_PROCESSES", "1"); err != nil {
		t.Fatalf("failed to set LYNX_MAX_PROCESSES: %v", err)
	}
	defer func() { _ = os.Unsetenv("LYNX_MAX_PROCESSES") }()

	mgr := manager.NewManager()

	specA := protocol.AppSpec{
		Version: 1,
		ID:      uuid.Must(uuid.NewV7()).String(),
		Name:    "app-a",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
	}

	if _, err := mgr.StartWithSpec(specA); err != nil {
		t.Fatalf("StartWithSpec(A) failed: %v", err)
	}

	specB := protocol.AppSpec{
		Version: 1,
		ID:      uuid.Must(uuid.NewV7()).String(),
		Name:    "app-b",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
	}

	if _, err := mgr.StartWithSpec(specB); err == nil {
		t.Fatal("expected error when starting beyond LYNX_MAX_PROCESSES, got nil")
	}
}

func TestManager_MaxProcessesLimitInvalidEnv(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lynx-mgr-maxproc-invalid")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	t.Setenv("XDG_CONFIG_HOME", tempDir)

	logDir := filepath.Join(tempDir, "lynx/logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", tempDir)

	if err := os.Setenv("LYNX_MAX_PROCESSES", "invalid"); err != nil {
		t.Fatalf("failed to set LYNX_MAX_PROCESSES: %v", err)
	}
	defer func() { _ = os.Unsetenv("LYNX_MAX_PROCESSES") }()

	mgr := manager.NewManager()

	specA := protocol.AppSpec{
		Version: 1,
		ID:      uuid.Must(uuid.NewV7()).String(),
		Name:    "app-a",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
	}

	if _, err := mgr.StartWithSpec(specA); err == nil {
		t.Fatal("expected error for invalid LYNX_MAX_PROCESSES")
	} else if !strings.HasPrefix(err.Error(), "ERR_LIMITS: invalid LYNX_MAX_PROCESSES") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManager_MaxProcessesLimitNonPositive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lynx-mgr-maxproc-nonpositive")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	t.Setenv("XDG_CONFIG_HOME", tempDir)

	logDir := filepath.Join(tempDir, "lynx/logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", tempDir)

	if err := os.Setenv("LYNX_MAX_PROCESSES", "0"); err != nil {
		t.Fatalf("failed to set LYNX_MAX_PROCESSES: %v", err)
	}
	defer func() { _ = os.Unsetenv("LYNX_MAX_PROCESSES") }()

	mgr := manager.NewManager()

	specA := protocol.AppSpec{
		Version: 1,
		ID:      uuid.Must(uuid.NewV7()).String(),
		Name:    "app-a",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
	}

	if _, err := mgr.StartWithSpec(specA); err == nil {
		t.Fatal("expected error for non-positive LYNX_MAX_PROCESSES")
	} else if !strings.HasPrefix(err.Error(), "ERR_LIMITS: LYNX_MAX_PROCESSES must be > 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}
