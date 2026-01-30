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
	"github.com/robfig/cron/v3"
)

// Process wraps an OS process with state tracking.
type Process struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	info          types.ProcessInfo
	spec          protocol.AppSpec
	stoppedByUser bool
	exitError     error
	startTime     time.Time
	logFiles      []*os.File
	metrics       metrics.Collector
	scheduler     *cron.Cron
	restartCount  int
	lastRestart   time.Time
}

// NewProcess creates a new process instance.
// It does not start the process.
func NewProcess(id string, spec protocol.AppSpec) (*Process, error) {
	name := spec.Name
	if name == "" {
		if spec.Exec.Type == "entry" {
			name = filepath.Base(spec.Exec.Entry)
		} else {
			name = filepath.Base(spec.Exec.Command)
		}
	}

	proc := &Process{
		spec: spec,
		info: types.ProcessInfo{
			ID:        id,
			Name:      name,
			Namespace: "default",
			Version:   "0.0.1",
			Mode:      "fork",
			State:     types.StateStopped,
			Watch:     false,
		},
	}

	// Initialize Scheduler if cron is present
	if spec.Cron != "" {
		proc.scheduler = cron.New()
		_, err := proc.scheduler.AddFunc(spec.Cron, func() {
			_ = proc.Restart()
		})
		if err != nil {
			return nil, fmt.Errorf("invalid cron schedule: %w", err)
		}
	}

	return proc, nil
}

// Start runs the process and spawns the monitor goroutine.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.info.State == types.StateRunning {
		return errors.New("process already started")
	}

	cmd, err := p.prepareCmd()
	if err != nil {
		return err
	}
	p.cmd = cmd

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

	// Start scheduler if not running
	if p.scheduler != nil {
		p.scheduler.Start()
	}

	go p.monitor()

	return nil
}

// Restart stops and starts the process.
func (p *Process) Restart() error {
	_ = p.Stop()
	// Allow some time for cleanup?
	time.Sleep(100 * time.Millisecond)
	return p.Start()
}

// prepareCmd constructs the exec.Cmd from spec.
func (p *Process) prepareCmd() (*exec.Cmd, error) {
	var cmd *exec.Cmd
	ctx := context.Background()

	// Prepare the command parts based on type
	var finalBin string
	var finalArgs []string

	switch p.spec.Exec.Type {
	case "command":
		if p.spec.Exec.Command == "" {
			return nil, os.ErrInvalid
		}
		finalBin = p.spec.Exec.Command
		finalArgs = p.spec.Exec.Args

	case "entry":
		if p.spec.Exec.Entry == "" || p.spec.Exec.Runtime == "" {
			return nil, errors.New("ERR_BAD_REQUEST: entry and runtime required")
		}
		rtParts := strings.Fields(p.spec.Exec.Runtime)
		if len(rtParts) == 0 {
			return nil, errors.New("ERR_BAD_REQUEST: invalid runtime")
		}
		finalBin = rtParts[0]
		finalArgs = append(rtParts[1:], p.spec.Exec.Entry)
		finalArgs = append(finalArgs, p.spec.Exec.Args...)

	default:
		return nil, errors.New("ERR_BAD_REQUEST: invalid exec type")
	}

	// Apply argument limits
	if len(finalArgs) > 256 {
		return nil, errors.New("ERR_LIMITS: too many arguments (max 256)")
	}

	// Handle Shell Execution
	if p.spec.Exec.Shell {
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

	if p.spec.Cwd != "" {
		// Security Hardening: Cwd Validation
		info, err := os.Stat(p.spec.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: invalid cwd: %w", err)
		}
		cmd.Dir = p.spec.Cwd
	}

	// Environment preparation
	env := os.Environ()

	// Load EnvFile if present
	if p.spec.EnvFile != "" {
		file, err := os.Open(p.spec.EnvFile)
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
	if len(p.spec.Env) > 0 {
		for k, v := range p.spec.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	cmd.Env = env

	// Stdio handling
	if err := p.setupLogs(cmd); err != nil {
		return nil, err
	}

	// Configure isolation
	runAs := protocol.RunAsPolicy{Mode: "self"}
	if p.spec.RunAs != nil {
		runAs = *p.spec.RunAs
	}
	if err := daemonRuntime.ConfigureProcessIsolation(cmd, runAs); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (p *Process) setupLogs(cmd *exec.Cmd) error {
	// Close previous log files if any
	for _, f := range p.logFiles {
		_ = f.Close()
	}
	p.logFiles = nil

	logs := p.spec.Logs
	if logs == nil {
		logs = &protocol.AppLogs{Mode: "inherit"}
	}

	if logs.Mode == "inherit" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return nil
	}

	// Determine Log Directory
	logDir := logs.Dir
	if logDir == "" {
		if os.Geteuid() == 0 {
			logDir = "/var/log/lynx"
		} else {
			home, _ := os.UserHomeDir()
			logDir = filepath.Join(home, ".local/state/lynx/logs")
		}
	}

	// Create per-app log directory: <base>/<uuid>/
	appLogDir := filepath.Join(logDir, p.info.ID)
	if err := os.MkdirAll(appLogDir, 0700); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	// Open Stdout
	stdoutPath := logs.Stdout
	if stdoutPath == "" {
		stdoutPath = "stdout.log"
	}
	// If relative, join with appLogDir
	if !filepath.IsAbs(stdoutPath) {
		stdoutPath = filepath.Join(appLogDir, stdoutPath)
	}

	fOut, err := os.OpenFile(stdoutPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open stdout log: %w", err)
	}
	p.logFiles = append(p.logFiles, fOut)
	cmd.Stdout = fOut

	// Open Stderr
	stderrPath := logs.Stderr
	if stderrPath == "" {
		stderrPath = "stderr.log"
	}
	if !filepath.IsAbs(stderrPath) {
		stderrPath = filepath.Join(appLogDir, stderrPath)
	}

	if stderrPath == stdoutPath {
		cmd.Stderr = fOut
	} else {
		fErr, err := os.OpenFile(stderrPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open stderr log: %w", err)
		}
		p.logFiles = append(p.logFiles, fErr)
		cmd.Stderr = fErr
	}

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
	p.exitError = err
	
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if p.stoppedByUser {
		p.info.State = types.StateStopped
		p.mu.Unlock()
		return
	}

	if err != nil {
		p.info.State = types.StateFailed
	} else {
		p.info.State = types.StateExited
	}
	p.info.PID = 0
	p.mu.Unlock()

	// Handle Restart
	p.handleRestart(exitCode)
}

func (p *Process) handleRestart(exitCode int) {
	restart := p.spec.Restart
	if restart == nil {
		restart = &protocol.AppRestart{Policy: "on-failure", MaxRetries: 10, BackoffMs: 2000, BackoffType: "expo"}
	}

	// Check StopOnExit
	for _, code := range restart.StopOnExit {
		if exitCode == code {
			return // Treat as clean exit
		}
	}
	// Default: if 0 is not in StopOnExit (and not empty), assume success is 0. 
	// But the user said "default includes 0". 
	// So if exitCode is 0, we generally don't restart unless policy is always.

	shouldRestart := false
	switch restart.Policy {
	case "always":
		shouldRestart = true
	case "on-failure":
		shouldRestart = exitCode != 0
	case "never":
		shouldRestart = false
	}

	if !shouldRestart {
		return
	}

	// Check Max Retries (windowed? No, just count. Reset on successful long run?)
	// For simplicity: simple counter. Reset if running > 10s?
	// User didn't specify reset logic. I'll implement simple count.
	p.mu.Lock()
	if time.Since(p.lastRestart) > 60*time.Second {
		p.restartCount = 0
	}
	p.restartCount++
	count := p.restartCount
	p.lastRestart = time.Now()
	p.mu.Unlock()

	if count > restart.MaxRetries {
		fmt.Printf("Process %s reached max retries\n", p.info.Name)
		return
	}

	// Backoff
	delay := time.Duration(restart.BackoffMs) * time.Millisecond
	if restart.BackoffType == "expo" {
		// 2^(count-1) * delay. O(1) calculation.
		shift := count - 1
		if shift > 30 {
			shift = 30 // Prevent overflow
		}
		if shift > 0 {
			delay = delay << shift
		}
		
		// Cap at 5 minutes
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
	} else if restart.BackoffType == "linear" {
		delay = time.Duration(count) * delay
	}

	time.Sleep(delay)
	
	// Restart
	_ = p.Start()
}

// Stop signals the process to terminate.
func (p *Process) Stop() error {
	p.mu.Lock()
	
	// Stop scheduler
	if p.scheduler != nil {
		p.scheduler.Stop()
	}

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
	
	// Add restart info to status? 
	// types.ProcessInfo might need update if we want to show restart count.
	// But I won't touch types.ProcessInfo unless needed.

	return p.info
}
