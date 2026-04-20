//go:build linux

// Package runtime owns the per-process isolation primitives used by the
// daemon when spawning children: the default `self` mode attaches a plain
// SysProcAttr, `dynamic` is handled by the systemd-run wrapper in
// manager.prepareIsolation, and `sandbox` is provided by WrapSandbox.
package runtime

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// ConfigureProcessIsolation attaches the SysProcAttr appropriate for the
// requested RunAs mode. It is a no-op for "self" (and unknown modes) because
// "dynamic" and "sandbox" are wrapped at a higher layer.
//
// Setpgid is always enabled so the spawned process becomes the leader of its
// own process group. That lets Stop() signal the whole group with kill(-pid),
// which in turn reaches every fork()+exec() descendant — without it, a
// supervised app whose child outlives its parent (next-server, gunicorn
// pre-fork, bash wrappers) would leak orphans on stop, leave the listening
// socket bound, and trigger EADDRINUSE on the next start.
func ConfigureProcessIsolation(cmd *exec.Cmd, runAs protocol.RunAsPolicy) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	switch runAs.Mode {
	case "self":
		return nil
	case "app_user", "explicit_user":
		// Reserved for future per-app uid/gid isolation.
		return errors.New(
			"ERR_UNSUPPORTED: run_as=" + runAs.Mode +
				" is not implemented yet; use 'dynamic' or 'sandbox'")
	default:
		return nil
	}
}
