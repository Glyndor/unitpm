//go:build linux

package manager

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
		_ = os.Unsetenv("XDG_CONFIG_HOME")
		_ = os.Unsetenv("XDG_STATE_HOME")
		_ = os.Unsetenv("HOME")
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

	id := uuid.Must(uuid.NewV7()).String()
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

	id := uuid.Must(uuid.NewV7()).String()
	spec := protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "cron-test",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"10"},
		},
		Cron: "@every 10s",
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

// TestStopKillsForkedChildren guards the gracefulKill → descendant-walk
// behaviour end-to-end. Without the walk a `bash -c 'sleep & wait'`
// wrapper would die but the backgrounded sleep would survive Stop(true),
// keeping any listening socket bound and tripping EADDRINUSE on the next
// Start — the exact bug reported for next-server / bun / gunicorn.
//
// The captured PID is sleep's own PID (bash's direct child); walking /proc
// backwards from there must find the supervised wrapper and the signal
// must reach sleep whether it ends up in the wrapper's pgroup or a
// relocated one, which is what `lynxpm stop` has to deliver in the field.
func TestStopKillsForkedChildren(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	id := uuid.Must(uuid.NewV7()).String()
	spec := protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "fork-test",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "bash",
			Args:    []string{"-c", "sleep 60 & echo $! > " + pidFile + "; wait"},
		},
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for the child PID to be written by the wrapper.
	deadline := time.Now().Add(2 * time.Second)
	var childPID int
	for {
		if b, err := os.ReadFile(pidFile); err == nil && len(b) > 0 {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(b)))
			if err == nil && childPID > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for child PID file %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}

	parentPID := p.Info().PID
	if parentPID == 0 {
		t.Fatal("expected parent PID to be set after Start")
	}

	if err := p.Stop(true); err != nil {
		t.Fatalf("Stop(true) failed: %v", err)
	}

	// Give the kernel a moment to deliver SIGTERM -> SIGCHLD reaping.
	time.Sleep(500 * time.Millisecond)

	if aliveAndRunning(parentPID) {
		t.Errorf("parent PID %d still alive after Stop", parentPID)
	}
	if aliveAndRunning(childPID) {
		t.Errorf("child PID %d still alive after Stop — process group not killed", childPID)
	}
}

// aliveAndRunning reports whether pid exists AND is not already a zombie.
// Zombies (State: Z in /proc/<pid>/status) hold no file descriptors, no
// sockets, and no memory — they are indistinguishable from "dead" for any
// purpose Stop cares about. Treating them as alive would false-positive
// in containers whose PID 1 is not a reaper (debian/ubuntu runner images
// launched without tini/dumb-init), where the supervised wrapper dies
// before it can wait() on its own child.
func aliveAndRunning(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "State:") {
			return !strings.Contains(line, "Z")
		}
	}
	return true
}

func TestCronEveryIntervalBounds(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	id := uuid.Must(uuid.NewV7()).String()

	_, err := NewProcess(id, protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "cron-too-fast",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
		Cron: "@every 1s",
	})
	if err == nil {
		t.Fatalf("expected error for too-small cron interval")
	}
	if !strings.Contains(err.Error(), "ERR_LIMITS: cron interval") {
		t.Fatalf("expected ERR_LIMITS cron interval error, got %v", err)
	}

	id = uuid.Must(uuid.NewV7()).String()
	_, err = NewProcess(id, protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "cron-too-slow",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
		Cron: "@every 48h",
	})
	if err == nil {
		t.Fatalf("expected error for too-large cron interval")
	}
	if !strings.Contains(err.Error(), "ERR_LIMITS: cron interval") {
		t.Fatalf("expected ERR_LIMITS cron interval error, got %v", err)
	}

	id = uuid.Must(uuid.NewV7()).String()
	if _, err := NewProcess(id, protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "cron-ok",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"1"},
		},
		Cron: "@every 10s",
	}); err != nil {
		t.Fatalf("expected @every 10s to be accepted, got %v", err)
	}
}

func TestManualRestartReenablesAutoRestart(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	mgr := NewManager()

	id := uuid.Must(uuid.NewV7()).String()
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
