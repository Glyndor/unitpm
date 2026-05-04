package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	daemonRuntime "github.com/Jaro-c/Lynx/internal/daemon/runtime"
	"github.com/Jaro-c/Lynx/internal/env"
	"github.com/Jaro-c/Lynx/internal/git"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/metrics"
	"github.com/Jaro-c/Lynx/internal/paths"
	"github.com/Jaro-c/Lynx/internal/types"
)

// Process represents a single managed application instance and its state.
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
	stdoutPath    string // cached for banner reopen after files closed
	stderrPath    string
	stdoutWriter  *timestampWriter // nil in inherit mode; held so the rotation ticker can drive it
	stderrWriter  *timestampWriter
	inRestart     bool // suppresses STARTED/STOPPED banners during Restart()
	metrics       metrics.Collector
	scheduler     *cron.Cron
	restartCount  int
	lastRestart   time.Time
	cancelRestart context.CancelFunc // cancels pending restart backoff goroutine
	watcher       *fileWatcher
}

// NewProcess creates a new process instance.
// It does not start the process.
func NewProcess(id string, spec protocol.AppSpec) (*Process, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, errors.New("invalid process ID: must be a valid UUID v4")
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
		ns = types.DefaultNamespace
	}

	proc := &Process{
		spec: spec,
		info: types.ProcessInfo{
			ID:        id,
			Name:      name,
			Namespace: ns,
			Version:   detectProjectVersion(spec.Cwd),
			Mode:      "fork",
			State:     types.StateStopped,
			Watch:     false,
		},
	}

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
			_ = proc.Restart()
		})
		if err != nil {
			return nil, fmt.Errorf("ERR_LIMITS: invalid cron schedule: %w", err)
		}
	}

	return proc, nil
}

// emitBanner writes a 3-line lifecycle marker to every currently-open log
// file. Caller must hold p.mu. No-op when logs are inherit mode.
func (p *Process) emitBanner(event, detail string) {
	for _, f := range p.logFiles {
		writeBanner(f, event, detail)
	}
}

// emitBannerByPath writes a banner by reopening cached log paths. Used
// after monitor() has closed p.logFiles (handleRestart). No-op for inherit
// mode (paths empty) or when reopen fails.
func (p *Process) emitBannerByPath(event, detail string) {
	seen := map[string]struct{}{}
	for _, path := range []string{p.stdoutPath, p.stderrPath} {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if err != nil {
			continue
		}
		writeBanner(f, event, detail)
		_ = f.Close()
	}
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

	if col, err := metrics.NewCollector(p.info.PID); err == nil {
		p.metrics = col
	}

	// Capture Git Metadata (if applicable)
	// We capture this at Start time so it reflects the version being executed.
	if p.spec.Cwd != "" {
		if info, err := git.GetInfo(p.spec.Cwd); err == nil {
			p.info.GitBranch = info.Branch
			p.info.GitCommit = info.Commit
			p.info.GitDirty = info.Dirty
		}
	}

	if p.scheduler != nil {
		p.scheduler.Start()
	}

	// Start file watcher if configured (after releasing lock to avoid deadlock
	// — the onChange callback calls Restart which acquires p.mu).
	watchEnabled := p.spec.Watch != nil && p.spec.Watch.Enabled && p.spec.Cwd != ""
	if watchEnabled {
		p.info.Watch = true
		if p.watcher != nil {
			p.watcher.Stop()
		}
		p.watcher = newFileWatcher(p.spec.Cwd, p.spec.Watch.Ignore, func() {
			go func() { _ = p.Restart() }()
		})
	}

	if !p.inRestart {
		p.emitBanner("STARTED", "")
	}

	go p.monitor()

	if watchEnabled {
		p.watcher.Start()
	}

	return nil
}

// Restart stops the process (if running) and starts it again.
// Increments the Restarts counter regardless of the trigger (manual via
// `lynxpm restart`, cron schedule, or failure-driven via handleRestart).
func (p *Process) Restart() error {
	return p.restartLocked(true)
}

// autoRestart is the failure-path equivalent of Restart(): same Stop→Start
// sequence, but emits no RESTARTED banner (handleRestart writes
// AUTO-RESTART instead) and lets Start emit STARTED so the new log files
// get a fresh boundary marker.
func (p *Process) autoRestart() error {
	return p.restartLocked(false)
}

// restartLocked is the shared body of Restart and autoRestart. emitBanner
// controls whether a RESTARTED banner is emitted and whether Start's own
// STARTED banner is suppressed (manual restart wants the RESTARTED marker
// alone; auto-restart leaves Start to write STARTED into the new file).
func (p *Process) restartLocked(emitBanner bool) error {
	p.mu.Lock()
	if p.noAutoRestart {
		p.mu.Unlock()
		return nil
	}
	p.info.Restarts++
	if emitBanner {
		p.inRestart = true
		p.emitBanner("RESTARTED", "")
	}
	p.mu.Unlock()

	if emitBanner {
		defer func() {
			p.mu.Lock()
			p.inRestart = false
			p.mu.Unlock()
		}()
	}

	_ = p.Stop(false) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)
	return p.Start()
}

// prepareCmd constructs the exec.Cmd from spec.
func (p *Process) prepareCmd() (*exec.Cmd, error) {
	ctx := context.Background()

	finalBin, finalArgs, err := p.resolveCommand()
	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	if p.spec.Exec.Shell {
		shellBin := "/bin/sh"
		shellArgs := make([]string, 1, 2)
		shellArgs[0] = "-c"
		var sb strings.Builder
		sb.WriteString(shellQuote(finalBin))
		for _, a := range finalArgs {
			sb.WriteByte(' ')
			sb.WriteString(shellQuote(a))
		}
		cmd = exec.CommandContext(ctx, shellBin, append(shellArgs, sb.String())...)
	} else {
		cmd = exec.CommandContext(ctx, finalBin, finalArgs...)
	}

	if p.spec.Cwd != "" {
		info, err := os.Stat(p.spec.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: invalid cwd: %w", err)
		}
		cmd.Dir = p.spec.Cwd
	}

	env, err := p.prepareEnv()
	if err != nil {
		return nil, err
	}
	cmd.Env = env

	if err := p.setupLogs(cmd); err != nil {
		return nil, err
	}

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

// shellQuote prevents shell metacharacter injection in sh -c command lines.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
	var envs []string

	if paths.IsSystemMode() {
		// System Mode: Whitelist to prevent leaking secrets (e.g. AWS_KEYS)
		allowed := map[string]struct{}{
			"PATH": {}, "LANG": {}, "TERM": {}, "TZ": {}, "TMPDIR": {},
			"USER": {}, "LOGNAME": {}, "SHELL": {}, "PWD": {},
			"XDG_DATA_HOME": {}, "XDG_CONFIG_HOME": {}, "XDG_STATE_HOME": {},
			"XDG_CACHE_HOME": {}, "XDG_RUNTIME_DIR": {},
		}

		for _, e := range os.Environ() {
			key := strings.SplitN(e, "=", 2)[0]
			_, allow := allowed[key]
			if !allow && strings.HasPrefix(key, "LC_") {
				allow = true
			}
			// Block dangerous loader variables even if somehow whitelisted.
			if strings.HasPrefix(key, "LD_") || strings.HasPrefix(key, "DYLD_") {
				allow = false
			}
			if allow {
				envs = append(envs, e)
			}
		}
	} else {
		envs = os.Environ()
	}

	// In dynamic isolation, systemd manages HOME — strip any inherited
	// HOME. Otherwise ensure HOME is present (system-mode whitelist drops
	// it, user mode usually inherits one). Single pass: filter inherited
	// HOME for the dynamic case while remembering whether one was seen,
	// then conditionally append the daemon's HOME for non-dynamic.
	isDynamic := p.spec.RunAs != nil && p.spec.RunAs.Mode == "dynamic"
	filtered := envs[:0]
	hasHome := false
	for _, e := range envs {
		if strings.HasPrefix(e, "HOME=") {
			hasHome = true
			if isDynamic {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	envs = filtered
	if !isDynamic && !hasHome {
		envs = append(envs, "HOME="+os.Getenv("HOME"))
	}

	if p.spec.EnvFile != "" {
		parsedEnv, err := env.ParseFile(p.spec.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("ERR_BAD_REQUEST: failed to parse env file: %w", err)
		}
		for k, v := range parsedEnv {
			envs = append(envs, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if len(p.spec.Env) > 0 {
		for k, v := range p.spec.Env {
			envs = append(envs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return envs, nil
}

func (p *Process) prepareIsolation(ctx context.Context, cmd *exec.Cmd) (*exec.Cmd, error) {
	runAs := protocol.RunAsPolicy{Mode: "self"}
	if p.spec.RunAs != nil {
		runAs = *p.spec.RunAs
	}

	if runAs.Mode == "sandbox" {
		lynxBin, err := p.getLynxBinary()
		if err != nil {
			return nil, fmt.Errorf("sandbox: locate lynxpm binary: %w", err)
		}
		opts := daemonRuntime.SandboxOptions{
			LynxBin: lynxBin,
			Cwd:     p.spec.Cwd,
		}
		if p.spec.Logs != nil {
			opts.LogDir = p.spec.Logs.Dir
		}
		if r := p.spec.Resources; r != nil {
			// CPUMaxPercent has no rlimit equivalent; it applies to dynamic
			// mode only. Memory and tasks translate directly.
			if r.MemoryMaxBytes > 0 {
				opts.Limits.MemoryBytes = uint64(r.MemoryMaxBytes)
			}
			if r.TasksMax > 0 {
				opts.Limits.MaxProcs = uint64(r.TasksMax)
			}
		}
		return daemonRuntime.WrapSandbox(ctx, cmd, opts)
	}

	if runAs.Mode == "dynamic" {
		// Secure Environment via Credentials
		credsDir := filepath.Join(paths.CredsDir, p.info.ID)
		if err := os.MkdirAll(credsDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create creds dir: %w", err)
		}

		envPath := filepath.Join(credsDir, "env")
		// Write envs to file
		// We use cmd.Env which contains merged envs
		envContent := strings.Join(cmd.Env, "\n")
		if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
			// Clean up the directory we just created to avoid secrets leaking on disk.
			_ = os.RemoveAll(credsDir)
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
			"-p", "ProtectProc=invisible",
			"-p", "LoadCredential=env:" + envPath, // Expose env as credential
			"--pipe",
			"--wait",
		}

		if p.spec.Cwd != "" {
			sdArgs = append(sdArgs, "-p", "WorkingDirectory="+p.spec.Cwd)
		}

		if r := p.spec.Resources; r != nil {
			if r.MemoryMaxBytes > 0 {
				sdArgs = append(sdArgs, "-p", fmt.Sprintf("MemoryMax=%d", r.MemoryMaxBytes))
			}
			if r.CPUMaxPercent > 0 {
				sdArgs = append(sdArgs, "-p", fmt.Sprintf("CPUQuota=%d%%", r.CPUMaxPercent))
			}
			if r.TasksMax > 0 {
				sdArgs = append(sdArgs, "-p", fmt.Sprintf("TasksMax=%d", r.TasksMax))
			}
		}

		sdArgs = append(sdArgs, "--")

		// Use _exec-env wrapper
		lynxBin, err := p.getLynxBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to locate lynxpm binary for env wrapper: %w", err)
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
		p.stdoutPath = ""
		p.stderrPath = ""
		p.stdoutWriter = nil
		p.stderrWriter = nil
		return nil
	}

	// Determine Log Directory
	var logsDir, stdout, stderr string
	if logs := p.spec.Logs; logs != nil {
		logsDir = logs.Dir
		stdout = logs.Stdout
		stderr = logs.Stderr
	}

	stdoutPath, stderrPath, err := paths.ResolveLogPaths(p.info.ID, logsDir, stdout, stderr)
	if err != nil {
		return err
	}
	p.stdoutPath = stdoutPath
	p.stderrPath = stderrPath

	// Create per-app log directory (stdoutPath/stderrPath are usually in the same dir)
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0700); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(stderrPath), 0700); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	// Size-based rotation — safety net for user mode where logrotate is
	// typically not configured.
	rotateIfLarge(stdoutPath)
	if stderrPath != stdoutPath {
		rotateIfLarge(stderrPath)
	}

	// rawFD is the perf-oriented opt-out: when timestamps are disabled, point
	// cmd.Stdout/Stderr at the *os.File directly. Go's exec then dup2()s the
	// fd into the child — no pipe, no io.Copy goroutine, no bufio buffer per
	// stream. We keep the timestampWriter alive only as a rotation handle
	// for the daemon-wide rotator; its data path stays unused. At N=100 this
	// saves roughly 200 goroutines and ~6 MB of pipe-copy buffers.
	rawFD := p.spec.Logs != nil && p.spec.Logs.Timestamp == "none"

	// Open Stdout — O_NOFOLLOW blocks a pre-placed symlink from redirecting
	// log writes to an arbitrary file owned by (or writable by) the daemon UID.
	logFlags := os.O_APPEND | os.O_CREATE | os.O_WRONLY | syscall.O_NOFOLLOW
	fOut, err := os.OpenFile(stdoutPath, logFlags, 0600)
	if err != nil {
		return fmt.Errorf("failed to open stdout log: %w", err)
	}
	p.logFiles = append(p.logFiles, fOut)
	p.stdoutWriter = newRotatingTimestampWriter(fOut, stdoutPath)
	if rawFD {
		cmd.Stdout = fOut
	} else {
		cmd.Stdout = p.stdoutWriter
	}

	// Open Stderr
	if stderrPath == stdoutPath {
		p.stderrWriter = p.stdoutWriter
		if rawFD {
			cmd.Stderr = fOut
		} else {
			cmd.Stderr = p.stdoutWriter
		}
	} else {
		fErr, err := os.OpenFile(stderrPath, logFlags, 0600)
		if err != nil {
			return fmt.Errorf("failed to open stderr log: %w", err)
		}
		p.logFiles = append(p.logFiles, fErr)
		p.stderrWriter = newRotatingTimestampWriter(fErr, stderrPath)
		if rawFD {
			cmd.Stderr = fErr
		} else {
			cmd.Stderr = p.stderrWriter
		}
	}

	return nil
}

// monitor waits for process exit and updates state.
func (p *Process) monitor() {
	err := p.cmd.Wait()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	p.mu.Lock()
	// Emit EXITED banner before closing files. Skipped for user-initiated
	// stop (STOPPED already written) and Restart() (RESTARTED suffices). The
	// daemon-wide rotation loop (Manager.rotateLoop) reads p.stdoutWriter
	// under p.mu and will see the nil-out below before its next tick, so no
	// per-process cancel is needed here.
	if !p.stoppedByUser && !p.inRestart {
		p.emitBanner("EXITED", fmt.Sprintf("code=%d", exitCode))
	}
	// Close log files under lock to prevent races with concurrent Start() calls.
	for _, f := range p.logFiles {
		_ = f.Close()
	}
	p.logFiles = nil
	p.stdoutWriter = nil
	p.stderrWriter = nil
	if p.watcher != nil {
		p.watcher.Stop()
	}
	p.exitError = err

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
	// If a user Restart() is in flight, it is already orchestrating Stop+Start
	// — do not race it with a second auto-restart goroutine.
	p.mu.Lock()
	inRestart := p.inRestart
	p.mu.Unlock()
	if inRestart {
		return
	}

	restart := p.spec.Restart
	if restart == nil {
		restart = &protocol.AppRestart{
			Policy:      "on-failure",
			MaxRetries:  10,
			BackoffMs:   2000,
			BackoffType: "expo",
		}
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
	if time.Since(p.lastRestart) > 60*time.Second {
		p.restartCount = 0
	}
	p.restartCount++
	// info.Restarts is incremented inside Restart() so all trigger paths
	// (manual, cron, failure) share one counter update.
	count := p.restartCount
	p.lastRestart = time.Now()
	p.mu.Unlock()

	if count > restart.MaxRetries {
		log.Printf("Process %s reached max retries (%d)", p.info.Name, restart.MaxRetries)
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

	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	if p.cancelRestart != nil {
		p.cancelRestart()
	}
	p.cancelRestart = cancel
	// Files are closed by monitor at this point; reopen by path to write
	// the AUTO-RESTART marker so the next iteration's STARTED banner has
	// context.
	p.emitBannerByPath("AUTO-RESTART", fmt.Sprintf("attempt=%d delay=%s", count, delay))
	p.mu.Unlock()

	go func() {
		select {
		case <-time.After(delay):
			_ = p.autoRestart() //nolint:errcheck
		case <-ctx.Done():
		}
	}()
}

// defaultStopTimeout is the time to wait after the stop signal before
// sending SIGKILL when the spec does not override it.
const defaultStopTimeout = 10 * time.Second

// StopSignalByName is the single source of truth for which signal names
// AppStop.Signal may carry. validateStop uses the key set, resolveStop
// uses the syscall value. SIGKILL / SIGSEGV / SIGSTOP are never exposed.
var StopSignalByName = map[string]syscall.Signal{
	"SIGTERM": syscall.SIGTERM,
	"SIGINT":  syscall.SIGINT,
	"SIGHUP":  syscall.SIGHUP,
	"SIGQUIT": syscall.SIGQUIT,
	"SIGUSR1": syscall.SIGUSR1,
	"SIGUSR2": syscall.SIGUSR2,
}

// resolveStop returns the signal and timeout to apply based on spec.Stop,
// falling back to SIGTERM / defaultStopTimeout. Unknown signals silently
// degrade to SIGTERM.
func (p *Process) resolveStop() (syscall.Signal, time.Duration) {
	sig := syscall.SIGTERM
	timeout := defaultStopTimeout
	if p.spec.Stop == nil {
		return sig, timeout
	}
	if name := p.spec.Stop.Signal; name != "" {
		if s, ok := StopSignalByName[name]; ok {
			sig = s
		}
	}
	if ms := p.spec.Stop.TimeoutMs; ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	return sig, timeout
}

// Stop terminates the process gracefully. It sends the configured stop
// signal first, waits up to the configured timeout for the process to
// exit, then sends SIGKILL if needed. If byUser is true, automatic
// restarts are disabled.
func (p *Process) Stop(byUser bool) error {
	p.mu.Lock()

	if p.scheduler != nil {
		p.scheduler.Stop()
	}

	// Cancel any pending restart backoff goroutine.
	if p.cancelRestart != nil {
		p.cancelRestart()
		p.cancelRestart = nil
	}

	// Stop file watcher.
	if p.watcher != nil {
		p.watcher.Stop()
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
		if !p.inRestart {
			p.emitBanner("STOPPED", "")
		}
	}
	proc := p.cmd.Process
	sig, timeout := p.resolveStop()
	p.mu.Unlock()

	if proc == nil {
		return nil
	}

	return gracefulKill(proc, sig, timeout)
}

// gracefulKill delivers stopSignal to the supervised process and every
// descendant discovered via /proc, then polls until the parent exits or
// timeout elapses (in which case the whole tree is force-killed).
func gracefulKill(proc *os.Process, stopSignal syscall.Signal, timeout time.Duration) error {
	if err := signalTree(proc, stopSignal); err != nil {
		return killTree(proc)
	}

	deadline := time.After(timeout)
	// 50ms is low enough that the common fast-exit path returns in
	// under a tick, while still staying well clear of a syscall storm
	// (kill(pid, 0) costs nothing but a permission check).
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return killTree(proc)
		case <-ticker.C:
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				return nil
			}
		}
	}
}

// signalTree snapshots descendants via walkDescendants *before* any
// signal is sent so orphans reparented to init after the parent dies
// don't escape discovery, then signals leaves → pgroup → parent.
func signalTree(proc *os.Process, sig syscall.Signal) error {
	descendants := walkDescendants(proc.Pid)
	debug := os.Getenv("LYNX_DEBUG_STOP") != ""
	if debug {
		log.Printf("stop: root=%d descendants=%v sig=%d", proc.Pid, descendants, sig)
	}

	for _, pid := range descendants {
		err := syscall.Kill(pid, sig)
		if debug && err != nil {
			log.Printf("stop: kill pid=%d sig=%d err=%v", pid, sig, err)
		}
	}

	gerr := syscall.Kill(-proc.Pid, sig)
	if debug && gerr != nil {
		log.Printf("stop: kill -pgrp=%d sig=%d err=%v", proc.Pid, sig, gerr)
	}
	if gerr != nil && !errors.Is(gerr, syscall.ESRCH) {
		return gerr
	}

	if err := proc.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
		if debug {
			log.Printf("stop: signal parent=%d err=%v", proc.Pid, err)
		}
		return err
	}
	return nil
}

// killTree is signalTree with SIGKILL hard-wired, same pre-collect order.
func killTree(proc *os.Process) error {
	descendants := walkDescendants(proc.Pid)
	for _, pid := range descendants {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	_ = syscall.Kill(-proc.Pid, syscall.SIGKILL)
	return proc.Kill()
}

// walkDescendants scans /proc once, builds the forward ppid→children
// adjacency, and returns every descendant of root via DFS. Output is
// deepest-first so leaves are signalled before their shell wrappers.
func walkDescendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	children := make(map[int][]int, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ppid, ierr := metrics.GetPPID(pid)
		if ierr != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}

	var out []int
	var dfs func(int)
	dfs = func(pid int) {
		for _, kid := range children[pid] {
			dfs(kid)
			out = append(out, kid)
		}
	}
	dfs(root)
	return out
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

// Tree returns per-PID stats for the root process and all its descendants.
// Returns nil if the process is not running or the platform is unsupported.
func (p *Process) Tree() []metrics.ChildStat {
	p.mu.Lock()
	pid := p.info.PID
	state := p.info.State
	p.mu.Unlock()

	if state != types.StateRunning && state != types.StateOnline {
		return nil
	}
	tree, _ := metrics.GetProcessTree(pid)
	return tree
}

// Spec returns a deep copy of the process spec, safe for external mutation.
func (p *Process) Spec() protocol.AppSpec {
	s := p.spec

	// Deep copy Exec.Args slice
	if len(p.spec.Exec.Args) > 0 {
		s.Exec.Args = make([]string, len(p.spec.Exec.Args))
		copy(s.Exec.Args, p.spec.Exec.Args)
	}

	// Deep copy Env map
	if len(p.spec.Env) > 0 {
		s.Env = make(map[string]string, len(p.spec.Env))
		for k, v := range p.spec.Env {
			s.Env[k] = v
		}
	}

	// Deep copy pointer fields
	if p.spec.Logs != nil {
		logsCopy := *p.spec.Logs
		s.Logs = &logsCopy
	}
	if p.spec.Restart != nil {
		restartCopy := *p.spec.Restart
		if len(p.spec.Restart.StopOnExit) > 0 {
			restartCopy.StopOnExit = make([]int, len(p.spec.Restart.StopOnExit))
			copy(restartCopy.StopOnExit, p.spec.Restart.StopOnExit)
		}
		s.Restart = &restartCopy
	}
	if p.spec.RunAs != nil {
		runAsCopy := *p.spec.RunAs
		s.RunAs = &runAsCopy
	}
	if p.spec.Watch != nil {
		watchCopy := *p.spec.Watch
		if len(p.spec.Watch.Ignore) > 0 {
			watchCopy.Ignore = make([]string, len(p.spec.Watch.Ignore))
			copy(watchCopy.Ignore, p.spec.Watch.Ignore)
		}
		s.Watch = &watchCopy
	}

	return s
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

// resetMetrics zeroes the user-visible Restarts counter and the internal
// backoff bucket without touching the running process. Useful after fixing
// a crash loop and wanting to observe stability from a clean baseline.
func (p *Process) resetMetrics() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.info.Restarts = 0
	p.restartCount = 0
	p.lastRestart = time.Time{}
}

func (p *Process) getLynxBinary() (string, error) {
	// 1. Prefer standard PATH lookup (safe for Debian /usr/bin installs)
	path, err := exec.LookPath("lynxpm")
	if err == nil {
		return path, nil
	}

	// 2. Fallback: adjacent to current binary (useful for dev/testing)
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		lynxPath := filepath.Join(dir, "lynxpm")
		if _, err := os.Stat(lynxPath); err == nil {
			return lynxPath, nil
		}
	}

	return "", errors.New("lynxpm binary not found in PATH or adjacent to daemon")
}
