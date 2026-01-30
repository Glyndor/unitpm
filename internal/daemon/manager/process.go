//go:build linux

// Package manager implements the core process management logic.
package manager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	daemonRuntime "github.com/Jaro-c/Lynx/internal/daemon/runtime"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/metrics"
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
	logFiles      []*os.File
	metrics       metrics.Collector
}

// NewProcess creates a new process instance.
// It does not start the process.
func NewProcess(id string, spec protocol.AppSpec) (*Process, error) {
	// Explicit execution: NO shell unless explicitly requested (not implemented here yet)
	// We handle "command" and "entry" types.

	var cmd *exec.Cmd
	ctx := context.Background()

	// Prepare the command parts based on type
	var finalBin string
	var finalArgs []string

	switch spec.Exec.Type {
	case "command":
		if spec.Exec.Command == "" {
			return nil, os.ErrInvalid
		}
		finalBin = spec.Exec.Command
		finalArgs = spec.Exec.Args

	case "entry":
		if spec.Exec.Entry == "" || spec.Exec.Runtime == "" {
			return nil, errors.New("ERR_BAD_REQUEST: entry and runtime required")
		}
		rtParts := strings.Fields(spec.Exec.Runtime)
		if len(rtParts) == 0 {
			return nil, errors.New("ERR_BAD_REQUEST: invalid runtime")
		}
		finalBin = rtParts[0]
		finalArgs = append(rtParts[1:], spec.Exec.Entry)
		finalArgs = append(finalArgs, spec.Exec.Args...)

	default:
		return nil, errors.New("ERR_BAD_REQUEST: invalid exec type")
	}

	// Apply argument limits
	if len(finalArgs) > 256 {
		return nil, errors.New("ERR_LIMITS: too many arguments (max 256)")
	}

	// Handle Shell Execution
	if spec.Exec.Shell {
		shellBin := "/bin/sh"
		shellArgs := []string{"-c"}

		// Construct command line
		cmdLine := finalBin
		if len(finalArgs) > 0 {
			cmdLine += " " + strings.Join(finalArgs, " ")
		}

		cmd = exec.CommandContext(ctx, shellBin, append(shellArgs, cmdLine)...)
	} else {
		cmd = exec.CommandContext(ctx, finalBin, finalArgs...)
	}

	if spec.Cwd != "" {
		// Security Hardening: Cwd Validation
		info, err := os.Stat(spec.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: invalid cwd: %w", err)
		}
		cmd.Dir = spec.Cwd
	}

	// Environment preparation
	// Inherit from OS, then EnvFile, then explicit Env
	env := os.Environ()

	// Load EnvFile if present
	if spec.EnvFile != "" {
		file, err := os.Open(spec.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: failed to open env file: %w", err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			env = append(env, line)
		}
		_ = file.Close()

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: failed to read env file: %w", err)
		}
	}

	// Apply explicit Env vars
	if len(spec.Env) > 0 {
		for k, v := range spec.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	
	cmd.Env = env

	// Stdio handling
	// Default to inherit if Logs is nil
	logMode := "inherit"
	if spec.Logs != nil {
		logMode = spec.Logs.Mode
	}

	if logMode == "inherit" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
	}
	// "pipe" and "file" are unsupported in this phase, handled by caller or ignored.

	// Configure isolation (platform specific)
	// TODO: Verify client identity via socket credentials before applying isolation policies.
	// Currently, identity is implicitly trusted if the client can connect.
	runAs := protocol.RunAsPolicy{Mode: "self"}
	if spec.RunAs != nil {
		runAs = *spec.RunAs
	}
	if err := daemonRuntime.ConfigureProcessIsolation(cmd, runAs); err != nil {
		return nil, err
	}

	name := spec.Name
	if name == "" {
		if spec.Exec.Type == "entry" {
			name = filepath.Base(spec.Exec.Entry)
		} else {
			name = filepath.Base(spec.Exec.Command)
		}
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

	if p.cmd.Process != nil {
		return errors.New("process already started")
	}

	p.startTime = time.Now()
	if err := p.cmd.Start(); err != nil {
		p.info.State = types.StateFailed
		return fmt.Errorf("failed to start process: %w", err)
	}

	p.info.PID = p.cmd.Process.Pid
	p.info.State = types.StateRunning
	p.exitError = nil
	p.stoppedByUser = false

	// Init metrics
	if col, err := metrics.NewCollector(p.info.PID); err == nil {
		p.metrics = col
	}

	go p.monitor()

	return nil
}

// monitor waits for process exit and updates state.
func (p *Process) monitor() {
	err := p.cmd.Wait()

	// Close log files
	for _, f := range p.logFiles {
		_ = f.Close()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.exitError = err
	if p.stoppedByUser {
		p.info.State = types.StateStopped
	} else if err != nil {
		p.info.State = types.StateFailed
	} else {
		p.info.State = types.StateExited
	}
	// Reset PID to indicate not running
	p.info.PID = 0
}

// Stop signals the process to terminate.
func (p *Process) Stop() error {
	p.mu.Lock()
	if p.info.State != types.StateRunning {
		p.mu.Unlock()
		return nil // Already stopped
	}
	p.stoppedByUser = true
	proc := p.cmd.Process
	p.mu.Unlock()

	if proc == nil {
		return nil
	}

	// Try graceful termination first (SIGTERM equivalent)
	// On Windows, Kill is the only option usually, but Go's Signal might map.
	// For now, simple Kill.
	return proc.Kill()
}

// Info returns the current process info.
func (p *Process) Info() types.ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Update uptime if running
	if p.info.State == types.StateRunning {
		p.info.Uptime = time.Since(p.startTime).Milliseconds()

		if p.metrics != nil {
			if m, err := p.metrics.Collect(); err == nil {
				p.info.CPU = m.CPUPercent
				p.info.Memory = m.MemoryBytes
			}
		}
	} else {
		p.info.Uptime = 0
		p.info.CPU = 0
		p.info.Memory = 0
	}

	return p.info
}
