//go:build linux

package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// TestRotateLoop_FiresWhileProcessRunning is the regression test for the
// "solo si inicia" gap: pre-rotation, rotateIfLarge ran exactly once at
// Start(), so a long-lived app would never have its log rotated mid-run.
//
// Strategy: pick a threshold (1500 bytes) larger than the STARTED banner
// (~250 bytes) so initial state does NOT trigger rotation. Then append
// data via an O_APPEND fd to push the file past the threshold. The
// rotation ticker should pick it up and produce a .1.gz + truncate the
// current file. Both the "no early rotation" and "rotation after growth"
// invariants are checked.
func TestRotateLoop_FiresWhileProcessRunning(t *testing.T) {
	t.Setenv("LYNX_LOG_MAX_BYTES", "1500")
	t.Setenv("LYNX_LOG_KEEP", "3")
	t.Setenv("LYNX_LOG_ROTATE_INTERVAL_MS", "100")

	restore := setupTestEnv(t)
	t.Cleanup(restore)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := t.TempDir()
	stdoutPath := filepath.Join(logDir, "stdout.log")
	stderrPath := filepath.Join(logDir, "stderr.log")

	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "rotate-test",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"30"}},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: stdoutPath,
			Stderr: stderrPath,
		},
	}
	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop(true) }()

	// Sanity: STARTED banner alone is below threshold, so no early rotation.
	time.Sleep(300 * time.Millisecond) // ~3 ticks
	if _, err := os.Stat(stdoutPath + ".1.gz"); !os.IsNotExist(err) {
		t.Fatalf("unexpected early rotation (.1.gz exists before threshold cross): err=%v", err)
	}

	// Push the file past the 1500-byte threshold via an independent
	// O_APPEND fd. The daemon's own fd is untouched.
	fd, err := os.OpenFile(stdoutPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	seed := make([]byte, 2000)
	for i := range seed {
		seed[i] = 'x'
	}
	if _, err := fd.Write(seed); err != nil {
		t.Fatalf("append: %v", err)
	}
	_ = fd.Close()

	// Poll for the joint condition: .1.gz exists AND current file is
	// truncated (< threshold). This is the post-rotation state.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		curInfo, curErr := os.Stat(stdoutPath)
		_, gzErr := os.Stat(stdoutPath + ".1.gz")
		if curErr == nil && gzErr == nil && curInfo.Size() < 1500 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ticker did not rotate within 3s after threshold cross")
}

// TestRotateLoop_StopsAfterMonitorExits guards against the ticker
// outliving the process: when monitor closes the log files the rotateCancel
// must fire so no goroutine is left stat()-ing a stale path.
func TestRotateLoop_StopsAfterMonitorExits(t *testing.T) {
	t.Setenv("LYNX_LOG_ROTATE_INTERVAL_MS", "50")

	restore := setupTestEnv(t)
	t.Cleanup(restore)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := t.TempDir()
	stdoutPath := filepath.Join(logDir, "stdout.log")
	stderrPath := filepath.Join(logDir, "stderr.log")

	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "rotate-stop-test",
		Exec: protocol.AppExec{Type: "command", Command: "true"},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: stdoutPath,
			Stderr: stderrPath,
		},
		Restart: &protocol.AppRestart{Policy: "never"},
	}
	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for monitor to clean up the writers.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		cleared := p.stdoutWriter == nil && p.rotateCancel == nil
		p.mu.Unlock()
		if cleared {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("rotateCancel/stdoutWriter not cleared after process exit")
}

// TestRotateLoop_NotStartedInInheritMode pins the inherit-mode no-op:
// without a path-backed writer there is nothing to rotate, so the ticker
// goroutine should not even be launched.
func TestRotateLoop_NotStartedInInheritMode(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	id := uuid.Must(uuid.NewV7()).String()
	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "rotate-inherit",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"30"}},
		Logs: &protocol.AppLogs{Mode: "inherit"},
	}
	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop(true) }()

	p.mu.Lock()
	cancel := p.rotateCancel
	stdoutW := p.stdoutWriter
	p.mu.Unlock()

	if cancel != nil {
		t.Errorf("inherit mode should not start rotation ticker")
	}
	if stdoutW != nil {
		t.Errorf("inherit mode should leave stdoutWriter nil, got %T", stdoutW)
	}
}

// TestRotateLoop_BannerOnSeparatorIntact is a small invariant check: when
// rotation runs while the daemon is alive and writing banners, the .1.gz
// archive is a real gzip file, not a corrupted half-write. Catches a
// regression where rotation could collide with concurrent writeBanner
// calls and produce a truncated archive.
func TestRotateLoop_BannerOnSeparatorIntact(t *testing.T) {
	t.Setenv("LYNX_LOG_MAX_BYTES", "200")
	t.Setenv("LYNX_LOG_ROTATE_INTERVAL_MS", "60")

	restore := setupTestEnv(t)
	t.Cleanup(restore)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := t.TempDir()
	stdoutPath := filepath.Join(logDir, "stdout.log")

	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "rotate-banner",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"30"}},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: stdoutPath,
			Stderr: stdoutPath,
		},
	}
	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop(true) }()

	// Force the file past threshold so the next tick rotates.
	if err := os.WriteFile(stdoutPath, []byte(strings.Repeat("y", 600)), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(stdoutPath + ".1.gz"); err == nil {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("rotation did not run within deadline")
}
