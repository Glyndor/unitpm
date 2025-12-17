package daemon

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/types"
)

// Process wraps an OS process with state tracking.
type Process struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	info          types.ProcessInfo
	stoppedByUser bool
	exitError     error
	startTime     time.Time
}

// NewProcess creates a new process instance.
// It does not start the process.
func NewProcess(id int, name, command string) (*Process, error) {
	// Simple command parsing (naive split)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, os.ErrInvalid
	}

	// Use CommandContext to satisfy linter, though we use Background for now.
	cmd := exec.CommandContext(context.Background(), parts[0], parts[1:]...)

	return &Process{
		cmd: cmd,
		info: types.ProcessInfo{
			ID:        id,
			Name:      name,
			Namespace: "default",
			Version:   "0.0.1",
			Mode:      "fork",
			State:     types.StateStopped,
			Watch:     false,
		},
	}, nil
}

// Start runs the process and spawns the monitor goroutine.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reset state in case of reuse (though exec.Cmd prevents easy reuse)
	p.stoppedByUser = false
	p.exitError = nil
	p.startTime = time.Now()

	if err := p.cmd.Start(); err != nil {
		p.info.State = types.StateFailed
		return err
	}

	p.info.PID = p.cmd.Process.Pid
	p.info.State = types.StateRunning

	// Spawn monitor goroutine
	go p.monitor()

	return nil
}

// monitor waits for the process to exit and updates state.
func (p *Process) monitor() {
	// Block until process exits.
	// This must be done outside the lock.
	err := p.cmd.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.exitError = err
	p.info.PID = 0 // Process is gone

	switch {
	case p.stoppedByUser:
		p.info.State = types.StateStopped
	case err != nil:
		p.info.State = types.StateFailed
	default:
		p.info.State = types.StateExited
	}
}

// Stop signals the process to stop.
func (p *Process) Stop() error {
	p.mu.Lock()
	p.stoppedByUser = true

	if p.cmd.Process == nil {
		p.mu.Unlock()
		return nil // Not started or already dead (though Process struct might stick around)
	}

	// Check if already exited to avoid sending signal to non-existent process?
	// os.Process.Signal handles this usually, or returns error.
	proc := p.cmd.Process
	p.mu.Unlock()

	// Send Interrupt first.
	// In a real supervisor, we'd wait and then Kill.
	// For minimal implementation, just Signal.
	return proc.Signal(os.Interrupt)
}

// Info returns a snapshot of the process state.
func (p *Process) Info() types.ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	info := p.info
	if info.State == types.StateRunning {
		info.Uptime = time.Since(p.startTime).Milliseconds()
	} else {
		info.Uptime = 0
	}
	return info
}
