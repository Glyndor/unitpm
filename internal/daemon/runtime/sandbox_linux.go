//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/cli/commands/execsandbox"
	"github.com/Jaro-c/Lynx/internal/daemon/runtime/landlock"
	"github.com/Jaro-c/Lynx/internal/daemon/runtime/rlimit"
)

// SandboxOptions configure the unprivileged sandbox wrapper.
type SandboxOptions struct {
	LynxBin string
	Cwd     string
	LogDir  string
	Limits  rlimit.Limits
	// Allow overrides the landlock default allowlist when non-empty.
	Allow []landlock.PathAccess
}

// WrapSandbox rewrites cmd to run under the unprivileged sandbox wrapper:
//
//  1. A new user+pid+mount namespace is entered; UID/GID map to 0 inside.
//  2. The wrapper binary (`lynxpm _exec-sandbox`) sets rlimits, applies
//     Landlock, and execve's the real target.
//
// No sudo is required.
func WrapSandbox(ctx context.Context, cmd *exec.Cmd, opts SandboxOptions) (*exec.Cmd, error) {
	if opts.LynxBin == "" {
		return nil, errors.New("sandbox: LynxBin not set")
	}
	if !landlock.Supported() {
		// Best-effort: continue without landlock but keep other primitives.
		// A future flag could force abort instead.
		_, _ = fmt.Fprintln(os.Stderr,
			"lynx: warning: kernel does not support Landlock; sandbox will be weaker")
	}

	cfg := execsandbox.Config{
		Cwd:     opts.Cwd,
		LogDir:  opts.LogDir,
		Allow:   opts.Allow,
		Limits:  opts.Limits,
		Command: cmd.Path,
		Args:    cmd.Args[1:],
	}
	payload, err := execsandbox.Serialize(cfg)
	if err != nil {
		return nil, fmt.Errorf("sandbox serialize: %w", err)
	}

	// Construct the wrapper command.
	wrapperArgs := execsandbox.WrapperCommand(opts.LynxBin)
	newCmd := exec.CommandContext(ctx, wrapperArgs[0], wrapperArgs[1:]...)
	newCmd.Stdout = cmd.Stdout
	newCmd.Stderr = cmd.Stderr
	newCmd.Stdin = cmd.Stdin

	// Propagate env plus the config blob.
	newCmd.Env = append(cmd.Env, execsandbox.ConfigEnvVar()+"="+payload)

	// User + PID + mount namespaces. UID/GID mapped to 0 inside so the
	// child "feels" like root but has no real privileges.
	uid := os.Getuid()
	gid := os.Getgid()
	// User + PID + mount namespaces at once. The mount namespace lets the
	// child try to remount /proc (best-effort — often blocked on modern
	// distros by locked mounts / AppArmor policies).
	newCmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
		},
		GidMappingsEnableSetgroups: false,
		Setpgid:                    true,
	}

	return newCmd, nil
}
