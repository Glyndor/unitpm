//! `ConfigureProcessIsolation` — attach the syscall attributes appropriate
//! for the requested `RunAsPolicy.Mode`.
//!
//! Mirrors `start_linux.go`. `self` (the default) gets a plain
//! `SysProcAttr` with `Setpgid` set; the daemon uses that to kill the
//! process group on `Stop`. `app_user` and `explicit_user` are reserved
//! for a later phase and surface `ERR_UNSUPPORTED`. `dynamic` and
//! `sandbox` are handled at higher layers (the systemd-run wrapper in
//! `manager.prepareIsolation` and [`super::sandbox`] respectively) and
//! fall through to the same plain `SysProcAttr`.

use crate::ipc::protocol::RunAsPolicy;

/// Per-process syscall attributes. Mirrors the Linux-only fields of
/// `syscall.SysProcAttr` that this module touches. Kept narrow on
/// purpose — the wider Go struct has fields (Credential, Ptrace, etc.)
/// the daemon does not use, and adding them here would invite drift.
#[derive(Debug, Default, Clone)]
pub struct ProcessAttr {
	/// `setpgid` — when true, the child becomes the leader of a new
	/// process group. Required so `Stop` can `kill(-pid)` the group.
	pub set_pgid: bool,
}

/// Errors returned when a `RunAsPolicy.Mode` is reserved for a later
/// phase. The Go implementation wraps the error in a string that starts
/// with `"ERR_UNSUPPORTED"`; the [`Display`](std::fmt::Display) impl
/// preserves the same prefix so callers can match on it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IsolationError {
	/// The mode that was rejected.
	pub mode: String,
}

impl std::fmt::Display for IsolationError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(
			f,
			"ERR_UNSUPPORTED: run_as={} is not implemented yet; use 'dynamic' or 'sandbox'",
			self.mode
		)
	}
}

impl std::error::Error for IsolationError {}

/// Attach the syscall attributes for `run_as.mode` to `cmd`. Returns the
/// populated [`ProcessAttr`] on success, or [`IsolationError`] when the
/// mode is reserved for a later phase. The Go implementation mutates an
/// `*exec.Cmd` in place; the Rust port returns the populated struct so
/// callers can compose it into a [`std::process::Command`] or other
/// spawn primitive.
pub fn configure_process_isolation(run_as: &RunAsPolicy) -> Result<ProcessAttr, IsolationError> {
	let attr = ProcessAttr {
		// Setpgid is enabled unconditionally so Stop can kill(-pid) the
		// whole group. This matches the Go implementation.
		set_pgid: true,
	};

	match run_as.mode.as_str() {
		"self" => Ok(attr),
		"app_user" | "explicit_user" => {
			// Reserved for future per-app uid/gid isolation.
			Err(IsolationError {
				mode: run_as.mode.clone(),
			})
		}
		// `dynamic` and `sandbox` are handled at a higher layer.
		_ => Ok(attr),
	}
}
