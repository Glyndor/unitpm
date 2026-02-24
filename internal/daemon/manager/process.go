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
	"github.com/Jaro-c/Lynx/internal/paths"
	"github.com/Jaro-c/Lynx/internal/types"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type Process struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	info          types.ProcessInfo
	spec          protocol.AppSpec
	stoppedByUser bool
	noAutoRestart bool
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
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("invalid process ID: must be a valid UUID v4")
	}

	name := spec.Name
	if name == "" {
		if spec.Exec.Type == "entry" {
			name = filepath.Base(spec.Exec.Entry)
		} else {
			name = filepath.Base(spec.Exec.Command)
		}
	}

	ns := spec.Namespace
	if ns == "" {
		ns = "default"
	}

	proc := &Process{
		spec: spec,
		info: types.ProcessInfo{
			ID:        id,
			Name:      name,
			Namespace: ns,
			Version:   "0.0.1",
			Mode:      "fork",
			State:     types.StateStopped,
			Watch:     false,
		},
	}

	// Initialize Scheduler if cron is present
	if spec.Cron != "" {
		if strings.HasPrefix(spec.Cron, "@every ") {
			durStr := strings.TrimSpace(strings.TrimPrefix(spec.Cron, "@every "))
			d, err := time.ParseDuration(durStr)
			if err != nil {
				return nil, fmt.Errorf("ERR_LIMITS: invalid cron interval: %w", err)
			}
			if d < 5*time.Second {
				return nil, errors.New("ERR_LIMITS: cron interval must be >= 5s")
			}
			if d > 24*time.Hour {
				return nil, errors.New("ERR_LIMITS: cron interval must be <= 24h")
			}
		}

		proc.scheduler = cron.New()
		_, err := proc.scheduler.AddFunc(spec.Cron, func() {
			_ = proc.Restart() //nolint:errcheck
		})
		if err != nil {
			return nil, fmt.Errorf("ERR_LIMITS: invalid cron schedule: %w", err)
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

	now := time.Now()
	p.startTime = now
	if err := p.cmd.Start(); err != nil {
		p.info.State = types.StateFailed
		return fmt.Errorf("failed to start process: %w", err)
	}

	p.info.PID = p.cmd.Process.Pid
	p.info.State = types.StateRunning
	if p.info.CreatedAt == "" {
		p.info.CreatedAt = now.Format(time.RFC3339)
	}
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

func (p *Process) Restart() error {
	p.mu.Lock()
	if p.noAutoRestart {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	_ = p.Stop(false) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)
	return p.Start()
}

// prepareCmd constructs the exec.Cmd from spec.
func (p *Process) prepareCmd() (*exec.Cmd, error) {
	ctx := context.Background()

	// 1. Prepare base command (binary + args)
	finalBin, finalArgs, err := p.resolveCommand()
	if err != nil {
		return nil, err
	}

	// 2. Handle Shell Execution
	var cmd *exec.Cmd
	if p.spec.Exec.Shell {
		shellBin := "/bin/sh"
		shellArgs := []string{"-c"}
		cmdLine := finalBin
		if len(finalArgs) > 0 {
			cmdLine += " " + strings.Join(finalArgs, " ")
		}
		cmd = exec.CommandContext(ctx, shellBin, append(shellArgs, cmdLine)...)
	} else {
		cmd = exec.CommandContext(ctx, finalBin, finalArgs...)
	}

	// 3. Set Cwd
	if p.spec.Cwd != "" {
		info, err := os.Stat(p.spec.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: invalid cwd: %w", err)
		}
		cmd.Dir = p.spec.Cwd
	}

	// 4. Prepare Environment
	env, err := p.prepareEnv()
	if err != nil {
		return nil, err
	}
	cmd.Env = env

	// 5. Stdio handling
	if err := p.setupLogs(cmd); err != nil {
		return nil, err
	}

	// 6. Configure isolation (wraps command if needed)
	cmd, err = p.prepareIsolation(ctx, cmd)
	if err != nil {
		// Close logs if isolation fails to prevent FD leak
		for _, f := range p.logFiles {
			_ = f.Close()
		}
		p.logFiles = nil
		return nil, err
	}

	return cmd, nil
}

func (p *Process) resolveCommand() (string, []string, error) {
	var finalBin string
	var finalArgs []string

	switch p.spec.Exec.Type {
	case "command":
		if p.spec.Exec.Command == "" {
			return "", nil, os.ErrInvalid
		}
		finalBin = p.spec.Exec.Command
		finalArgs = p.spec.Exec.Args

	case "entry":
		if p.spec.Exec.Entry == "" || p.spec.Exec.Runtime == "" {
			return "", nil, errors.New("ERR_BAD_REQUEST: entry and runtime required")
		}
		rtParts := strings.Fields(p.spec.Exec.Runtime)
		if len(rtParts) == 0 {
			return "", nil, errors.New("ERR_BAD_REQUEST: invalid runtime")
		}
		finalBin = rtParts[0]
		// Append runtime args + entry + user args
		finalArgs = append(finalArgs, rtParts[1:]...)
		finalArgs = append(finalArgs, p.spec.Exec.Entry)
		finalArgs = append(finalArgs, p.spec.Exec.Args...)

	default:
		return "", nil, errors.New("ERR_BAD_REQUEST: invalid exec type")
	}

	if len(finalArgs) > 256 {
		return "", nil, errors.New("ERR_LIMITS: too many arguments (max 256)")
	}

	return finalBin, finalArgs, nil
}

func (p *Process) prepareEnv() ([]string, error) {
	var env []string
	uid := os.Geteuid()

	// 1. Base Environment
	if uid == 0 {
		// System Mode: Whitelist to prevent leaking secrets (e.g. AWS_KEYS)
		allowed := []string{
			"PATH", "LANG", "TERM", "TZ", "TMPDIR",
			"USER", "LOGNAME", "SHELL", "PWD",
			"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR",
		}

		sysEnv := os.Environ()
		for _, e := range sysEnv {
			key := strings.SplitN(e, "=", 2)[0]
			allow := false
			for _, a := range allowed {
				if key == a {
					allow = true
					break
				}
			}
			if !allow && strings.HasPrefix(key, "LC_") {
				allow = true
			}
			if allow {
				env = append(env, e)
			}
		}
	} else {
		// User Mode: Inherit full environment
		env = os.Environ()
	}

	// 2. Handle HOME
	// In dynamic isolation, systemd manages HOME. Do not inject daemon's HOME.
	isDynamic := p.spec.RunAs != nil && p.spec.RunAs.Mode == "dynamic"

	if isDynamic {
		// Filter out HOME if it exists (e.g. from user mode inheritance)
		filtered := env[:0]
		for _, e := range env {
			if !strings.HasPrefix(e, "HOME=") {
				filtered = append(filtered, e)
			}
		}
		env = filtered
	} else {
		// If not dynamic, ensure HOME is present (especially for system mode where we didn't whitelist it)
		// Check if HOME is already there
		hasHome := false
		for _, e := range env {
			if strings.HasPrefix(e, "HOME=") {
				hasHome = true
				break
			}
		}
		if !hasHome {
			env = append(env, "HOME="+os.Getenv("HOME"))
		}
	}

	// 3. Env File
	if p.spec.EnvFile != "" {
		file, err := os.Open(p.spec.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: failed to open env file: %w", err)
		}
		defer func() { _ = file.Close() }() //nolint:errcheck

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			env = append(env, line)
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: failed to read env file: %w", err)
		}
	}

	if len(p.spec.Env) > 0 {
		for k, v := range p.spec.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return env, nil
}

func (p *Process) prepareIsolation(ctx context.Context, cmd *exec.Cmd) (*exec.Cmd, error) {
	runAs := protocol.RunAsPolicy{Mode: "self"}
	if p.spec.RunAs != nil {
		runAs = *p.spec.RunAs
	}

	if runAs.Mode == "dynamic" {
		// Secure Environment via Credentials
		credsDir := filepath.Join("/var/lib/lynx/creds", p.info.ID)
		if err := os.MkdirAll(credsDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create creds dir: %w", err)
		}

		envPath := filepath.Join(credsDir, "env")
		// Write envs to file
		// We use cmd.Env which contains merged envs
		envContent := strings.Join(cmd.Env, "\n")
		if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
			return nil, fmt.Errorf("failed to write env creds: %w", err)
		}

		// Wrap with systemd-run
		sdArgs := []string{
			"--unit=lynx-app-" + p.info.ID,
			"--description=" + p.info.Name,
			"-p", "DynamicUser=yes",
			"-p", "NoNewPrivileges=yes",
			"-p", "PrivateTmp=yes",
			"-p", "ProtectSystem=strict",
			"-p", "ProtectHome=yes",
			"-p", "LoadCredential=env:" + envPath, // Expose env as credential
			"--pipe",
			"--wait",
		}

		if p.spec.Cwd != "" {
			sdArgs = append(sdArgs, "-p", "WorkingDirectory="+p.spec.Cwd)
		}

		sdArgs = append(sdArgs, "--")

		// Use _exec-env wrapper
		lynxBin, err := p.getLynxBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to locate lynx binary for env wrapper: %w", err)
		}
		sdArgs = append(sdArgs, lynxBin, "_exec-env")

		sdArgs = append(sdArgs, cmd.Path)
		sdArgs = append(sdArgs, cmd.Args[1:]...)

		newCmd := exec.CommandContext(ctx, "systemd-run", sdArgs...)
		// Do NOT pass host env to systemd-run to avoid leaking secrets in process tree
		// newCmd.Env = cmd.Env
		newCmd.Stdout = cmd.Stdout
		newCmd.Stderr = cmd.Stderr
		return newCmd, nil
	}

	if err := daemonRuntime.ConfigureProcessIsolation(cmd, runAs); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Process) setupLogs(cmd *exec.Cmd) error {
	for _, f := range p.logFiles {
		_ = f.Close() //nolint:errcheck
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
	stdoutPath, stderrPath, err := paths.ResolveLogPaths(&p.spec)
	if err != nil {
		return err
	}

	// Create per-app log directory (stdoutPath/stderrPath are usually in the same dir)
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0700); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(stderrPath), 0700); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	// Open Stdout
	fOut, err := os.OpenFile(stdoutPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open stdout log: %w", err)
	}
	p.logFiles = append(p.logFiles, fOut)
	cmd.Stdout = fOut

	// Open Stderr
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

	for _, f := range p.logFiles {
		_ = f.Close() //nolint:errcheck
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
		p.info.PID = 0
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

	p.handleRestart(exitCode)
}

func (p *Process) handleRestart(exitCode int) {
	restart := p.spec.Restart
	if restart == nil {
		restart = &protocol.AppRestart{Policy: "on-failure", MaxRetries: 10, BackoffMs: 2000, BackoffType: "expo"}
	}

	for _, code := range restart.StopOnExit {
		if exitCode == code {
			return
		}
	}

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

	p.mu.Lock()
	p.info.State = types.StateRestarting
	p.mu.Unlock()

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
		p.mu.Lock()
		p.info.State = types.StateFailed
		p.mu.Unlock()
		return
	}

	delay := time.Duration(restart.BackoffMs) * time.Millisecond
	switch restart.BackoffType {
	case "expo":
		shift := count - 1
		if shift > 30 {
			shift = 30
		}
		if shift > 0 {
			delay <<= shift
		}
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
	case "linear":
		delay = time.Duration(count) * delay
	}

	go func() {
		time.Sleep(delay)
		_ = p.Restart() //nolint:errcheck
	}()
}

func (p *Process) Stop(byUser bool) error {
	p.mu.Lock()

	if p.scheduler != nil {
		p.scheduler.Stop()
	}

	if byUser {
		p.noAutoRestart = true
	}

	if p.info.State != types.StateRunning {
		if byUser {
			p.info.State = types.StateStopped
			p.info.PID = 0
		}
		p.mu.Unlock()
		return nil
	}
	if byUser {
		p.stoppedByUser = true
		p.info.State = types.StateStopped
		p.info.PID = 0
	}
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

// Spec returns the process spec.
func (p *Process) Spec() protocol.AppSpec {
	return p.spec
}

// ResetBackoff resets the restart counter and backoff timer.
// This should be called on manual restart.
func (p *Process) ResetBackoff() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restartCount = 0
	p.lastRestart = time.Time{}
	p.noAutoRestart = false
}

func (p *Process) getLynxBinary() (string, error) {
	// 1. Prefer standard PATH lookup (safe for Debian /usr/bin installs)
	path, err := exec.LookPath("lynx")
	if err == nil {
		return path, nil
	}

	// 2. Fallback: adjacent to current binary (useful for dev/testing)
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		lynxPath := filepath.Join(dir, "lynx")
		if _, err := os.Stat(lynxPath); err == nil {
			return lynxPath, nil
		}
	}

	return "", fmt.Errorf("lynx binary not found in PATH or adjacent to daemon")
}
