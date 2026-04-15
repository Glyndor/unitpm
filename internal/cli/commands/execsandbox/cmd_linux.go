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
	os.Unsetenv(envConfig) // Don't leak into the child.

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("invalid sandbox config: %w", err)
	}
	if cfg.Command == "" {
		return errors.New("sandbox config missing command")
	}

	// Remount /proc so the new PID namespace sees only its own processes
	// (ps, top, /proc/<pid>/... all become namespace-local). Requires the
	// CLONE_NEWNS | CLONE_NEWPID flags set by the parent. Best-effort: if it
	// fails we continue — the sandbox still has landlock+rlimit+user-ns.
	//
	// Mark the root mount as private first so our unshare's mount namespace
	// doesn't propagate back to the host (systemd defaults to shared).
	_ = unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, "")
	// Unmount the inherited /proc so we can cover it with a fresh one
	// scoped to the new PID namespace. MNT_DETACH is used so we don't
	// block on any open descriptors held by the parent.
	_ = unix.Unmount("/proc", unix.MNT_DETACH)
	mountFlags := uintptr(unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC)
	if err := unix.Mount("proc", "/proc", "proc", mountFlags, ""); err != nil {
		fmt.Fprintf(os.Stderr, "lynx: warning: could not remount /proc in sandbox: %v\n", err)
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
		Usage:       "lynx _exec-sandbox",
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
