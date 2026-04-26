//go:build linux

// Package execsandbox is the child wrapper used by --isolation sandbox.
//
// Flow: parent spawns this wrapper inside a new user+pid+net namespace. The
// wrapper then applies the final hardening (rlimits, landlock, no-new-privs)
// and execve's the real target. All configuration is passed via the
// LYNX_SANDBOX_* environment variables so the wrapper needs no flags.
package execsandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/daemon/runtime/landlock"
	"github.com/Jaro-c/Lynx/internal/daemon/runtime/rlimit"
)

const (
	envConfig = "LYNX_SANDBOX_CONFIG"
)

// Config is marshaled as JSON into LYNX_SANDBOX_CONFIG by the daemon.
type Config struct {
	Cwd     string                `json:"cwd"`
	LogDir  string                `json:"log_dir,omitempty"`
	Allow   []landlock.PathAccess `json:"allow,omitempty"`
	Limits  rlimit.Limits         `json:"limits"`
	Command string                `json:"command"`
	Args    []string              `json:"args"`
}

// Run is invoked when a sandboxed process starts. It performs the final
// hardening steps and execve's into the target command. It does not return
// on success.
func Run(args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	raw := os.Getenv(envConfig)
	if raw == "" {
		return errors.New("LYNX_SANDBOX_CONFIG not set")
	}
	_ = os.Unsetenv(envConfig)

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("invalid sandbox config: %w", err)
	}
	if cfg.Command == "" {
		return errors.New("sandbox config missing command")
	}
	if cfg.Cwd != "" && !filepath.IsAbs(cfg.Cwd) {
		return fmt.Errorf("sandbox cwd must be absolute: %q", cfg.Cwd)
	}

	// Unconditional: applies even if landlock or mount steps below fail,
	// closing the setuid-binary escape hatch on kernels that don't support
	// landlock.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}

	// Mark root private first. If this fails, abort — a subsequent unmount
	// of /proc would propagate back to the host and break every process on
	// the box (systemd defaults to shared mount propagation).
	if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make-rprivate /: %w", err)
	}
	// Remount /proc so the new PID namespace sees only its own processes.
	// MNT_DETACH avoids blocking on descriptors held by the parent.
	_ = unix.Unmount("/proc", unix.MNT_DETACH)
	procFlags := uintptr(unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC)
	if err := unix.Mount("proc", "/proc", "proc", procFlags, ""); err != nil {
		fmt.Fprintf(os.Stderr, "lynxpm: warning: could not remount /proc in sandbox: %v\n", err)
	}

	// Per-sandbox private /tmp. Without this, two sandboxes of the same host
	// user share /tmp (landlock grants RWX there by default) — sandbox A can
	// drop a binary for sandbox B to execute.
	tmpFlags := uintptr(unix.MS_NOSUID | unix.MS_NODEV)
	if err := unix.Mount("tmpfs", "/tmp", "tmpfs", tmpFlags, "mode=1777"); err != nil {
		return fmt.Errorf("mount private /tmp: %w", err)
	}

	if cfg.Cwd != "" {
		if err := os.Chdir(cfg.Cwd); err != nil {
			return fmt.Errorf("chdir %q: %w", cfg.Cwd, err)
		}
	}

	if err := rlimit.Apply(cfg.Limits); err != nil {
		return fmt.Errorf("rlimit: %w", err)
	}

	// Landlock: explicit allow list takes priority; fall back to sensible
	// defaults if the daemon didn't supply one.
	rs := landlock.Ruleset{Allow: cfg.Allow}
	if len(rs.Allow) == 0 {
		rs = landlock.SensibleDefaults(cfg.Cwd, cfg.LogDir)
	}
	if err := landlock.Apply(rs); err != nil {
		return fmt.Errorf("landlock: %w", err)
	}

	// Resolve binary path before restricting so we get a meaningful error.
	path, err := exec.LookPath(cfg.Command)
	if err != nil {
		return fmt.Errorf("command not found: %s", cfg.Command)
	}

	argv := append([]string{path}, cfg.Args...)
	env := os.Environ()
	if err := syscall.Exec(path, argv, env); err != nil {
		return fmt.Errorf("execve: %w", err)
	}
	return nil
}

// Serialize returns the JSON encoding of a config suitable for LYNX_SANDBOX_CONFIG.
func Serialize(c Config) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ConfigEnvVar returns the env var name used to pass config.
func ConfigEnvVar() string { return envConfig }

// GetSpec describes the internal command for registry purposes.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "_exec-sandbox",
		Description: "Internal child wrapper for --isolation sandbox (no direct use)",
		Usage:       "lynxpm _exec-sandbox",
		Hidden:      true,
	}
}

// PrintHelp prints the stub help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}

// WrapperCommand returns the binary + subcommand tokens that the parent
// should invoke.
func WrapperCommand(lynxBin string) []string {
	return []string{lynxBin, "_exec-sandbox"}
}

// ShellQuote trivially quotes a list of strings for diagnostics/logs only.
func ShellQuote(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p)
	}
	return b.String()
}
