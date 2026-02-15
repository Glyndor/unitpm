//go:build linux

package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/types"
)

// setupTestEnv overrides HOME to prevent accidental fallback to real user directories.
func setupTestEnv(t *testing.T) func() {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "lynx-lifecycle-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	restore := func() {
		_ = os.RemoveAll(tempDir)
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_STATE_HOME")
		os.Unsetenv("HOME")
	}

	logDir := filepath.Join(tempDir, "lynx", "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		restore()
		t.Fatalf("failed to create log dir: %v", err)
	}

	if err := os.Setenv("XDG_CONFIG_HOME", tempDir); err != nil {
		restore()
		t.Fatalf("failed to set XDG_CONFIG_HOME: %v", err)
	}
	if err := os.Setenv("XDG_STATE_HOME", tempDir); err != nil {
		restore()
		t.Fatalf("failed to set XDG_STATE_HOME: %v", err)
	}
	if err := os.Setenv("HOME", tempDir); err != nil {
		restore()
		t.Fatalf("failed to set HOME: %v", err)
	}

	return restore
}

func TestStopDuringBackoffPreventsRestart(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	id := uuid.NewString()
	spec := protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "backoff-test",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "false",
		},
		Restart: &protocol.AppRestart{
			Policy:     "on-failure",
			MaxRetries: 3,
			BackoffMs:  200,
		},
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		info := p.Info()
		if info.State == types.StateRestarting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for restarting state, got %s", info.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := p.Stop(true); err != nil {
		t.Fatalf("Stop(true) failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	info := p.Info()
	if info.State == types.StateRunning || info.State == types.StateRestarting {
		t.Fatalf("expected stopped state after Stop(true), got %s", info.State)
	}
	if info.PID != 0 {
		t.Fatalf("expected PID 0 after Stop(true), got %d", info.PID)
	}
}

func TestCronRespectsNoAutoRestart(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	id := uuid.NewString()
	spec := protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "cron-test",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"10"},
		},
		Cron: "@every 100ms",
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		info := p.Info()
		if info.State == types.StateRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for running state, got %s", info.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := p.Stop(true); err != nil {
		t.Fatalf("Stop(true) failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	info := p.Info()
	if info.State == types.StateRunning || info.State == types.StateRestarting {
		t.Fatalf("expected non-running state after Stop(true), got %s", info.State)
	}
	if info.PID != 0 {
		t.Fatalf("expected PID 0 after Stop(true), got %d", info.PID)
	}
}

func TestManualRestartReenablesAutoRestart(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	mgr := NewManager()

	id := uuid.NewString()
	spec := protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "restart-test",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sh",
			Args:    []string{"-c", "sleep 0.2; exit 1"},
		},
		Restart: &protocol.AppRestart{
			Policy:     "on-failure",
			MaxRetries: 2,
			BackoffMs:  100,
		},
	}

	if _, err := mgr.StartWithSpec(spec); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		proc, ok := mgr.Get(id)
		if !ok {
			t.Fatal("process not found")
		}
		info := proc.Info()
		if info.State == types.StateRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for running state, got %s", info.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := mgr.Stop(id); err != nil {
		t.Fatalf("Manager.Stop failed: %v", err)
	}

	if err := mgr.Restart(id); err != nil {
		t.Fatalf("Manager.Restart failed: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	seenRestarting := false
	for !time.Now().After(deadline) {
		proc, ok := mgr.Get(id)
		if !ok {
			break
		}
		info := proc.Info()
		if info.State == types.StateRestarting {
			seenRestarting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !seenRestarting {
		t.Fatalf("expected restarting state after manual restart")
	}
}
