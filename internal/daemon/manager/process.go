//go:build linux

package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/daemon/runtime"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
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
func NewProcess(id int, spec protocol.StartSpec) (*Process, error) {
	// Explicit execution: NO shell, NO strings.Fields
	if spec.Cmd == "" {
		return nil, os.ErrInvalid
	}

	// Security Hardening: Max Args Limit
	if len(spec.Args) > 256 {
		return nil, errors.New("ERR_LIMITS: too many arguments (max 256)")
	}

	// Use CommandContext to satisfy linter, though we use Background for now.
	// SECURITY: Relative paths in spec.Cmd are resolved using the daemon's PATH.
	cmd := exec.CommandContext(context.Background(), spec.Cmd, spec.Args...)

	if spec.Cwd != "" {
		// Security Hardening: Cwd Validation
		info, err := os.Stat(spec.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: invalid cwd: %w", err)
		}
		cmd.Dir = spec.Cwd
	}

	// Environment: Inherit from OS and append/overwrite with spec.Env
	// This ensures PATH and other system vars are present.
	// SECURITY: We use an inherit+overlay policy. spec.Env is validated for limits in the handler.
	if len(spec.Env) > 0 {
		env := os.Environ()
		for k, v := range spec.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Stdio handling
	if spec.Stdio == "inherit" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
	}
	// "pipe" and "file" are unsupported in this phase, handled by caller or ignored.

	// Configure isolation (platform specific)
	// TODO: Verify client identity via socket credentials before applying isolation policies.
	// Currently, identity is implicitly trusted if the client can connect.
	if err := runtime.ConfigureProcessIsolation(cmd, spec.RunAs); err != nil {
		return nil, err
	}

	name := spec.Name
	if name == "" {
		name = filepath.Base(spec.Cmd)
	}

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
