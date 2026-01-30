//go:build linux

package manager_test

import (
	"os"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/types"
)

func TestProcess_RestartNonBlocking(t *testing.T) {
	// 1. Setup a process that fails immediately
	spec := protocol.AppSpec{
		ID:      "test-restart-non-blocking",
		Name:    "test-restart",
		Version: 1,
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sh",
			Args:    []string{"-c", "exit 1"},
		},
		Restart: &protocol.AppRestart{
			Policy:      "on-failure",
			MaxRetries:  3,
			BackoffMs:   100, // Short backoff for test
			BackoffType: "constant",
		},
		Logs: &protocol.AppLogs{
			Mode: "inherit",
		},
	}

	// Mock XDG_STATE_HOME for log dir creation (if needed)
	tempDir, err := os.MkdirTemp("", "lynx-test-logs")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Setenv("XDG_STATE_HOME", tempDir)

	proc, err := manager.NewProcess(spec.ID, spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}

	// 2. Start the process
	start := time.Now()
	if err := proc.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	duration := time.Since(start)

	// Verify Start() was non-blocking (should be very fast)
	if duration > 50*time.Millisecond {
		t.Errorf("Start() took too long: %v", duration)
	}

	// 3. Verify it enters Running state
	info := proc.Info()
	if info.State != types.StateRunning && info.State != types.StateExited && info.State != types.StateFailed {
		t.Errorf("Unexpected state: %v", info.State)
	}

	// 4. Wait for restart (Backoff 100ms + overhead)
	// It should exit immediately, enter handleRestart, sleep 100ms, then restart.
	time.Sleep(200 * time.Millisecond)
	
	_ = proc.Stop()
}

func TestProcess_SupervisionScheduling(t *testing.T) {
	// Test that supervision doesn't block
	spec := protocol.AppSpec{
		ID:      "test-supervision",
		Name:    "test-supervision",
		Version: 1,
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"0.5"},
		},
		Restart: &protocol.AppRestart{
			Policy:      "always",
			MaxRetries:  1,
			BackoffMs:   100,
			BackoffType: "linear",
		},
	}

	proc, err := manager.NewProcess(spec.ID, spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}

	t0 := time.Now()
	err = proc.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if time.Since(t0) > 100*time.Millisecond {
		t.Error("Start() blocked too long")
	}

	// Cleanup
	go func() {
		time.Sleep(1 * time.Second)
		_ = proc.Stop()
	}()
}
