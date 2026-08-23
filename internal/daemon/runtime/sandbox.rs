//! Unprivileged sandbox wrapper — assembles the per-process hardening
//! primitives (user/PID/mount namespaces + Landlock + rlimits) into a
//! wrapper command that the parent spawns and the wrapper execs.
//!
//! Mirrors `sandbox_linux.go`. The Rust port is API-incompatible with the
//! Go version because:
//!
//! 1. The env var carrying the wrapper config is `UNITPM_SANDBOX_CONFIG`,
//!    not the dead-brand name the Go tree still carries in its wrapper
//!    command (out of scope for this phase). The Rust test asserts on the
//!    Rust-side name because the test is a Rust-side unit test, not a
//!    Go/Rust integration test. When phase 7 deletes the Go wrapper and
//!    lands a Rust one, both ends will use this new name.
//!
//! 2. The wrapper subcommand is `_exec-sandbox` for the same reason — it
//!    matches the current Go `execsandbox.GetSpec().Name` until the Rust
//!    wrapper lands.
//!
//! 3. The struct that holds the wrapped command is `WrappedCommand`, not
//!    `*exec.Cmd`, because the test inspects the fields directly rather
//!    than via an OS-side descriptor. The fields are identical to what a
//!    `*exec.Cmd` would carry for the same input.
//!
//! The JSON payload format inside `UNITPM_SANDBOX_CONFIG` is byte-for-byte
//! compatible with the Go wrapper's config struct so that the existing
//! wrapper can consume it while phase 7 has not yet landed. Field names
//! (`cwd`, `log_dir`, `command`, `args`, `allow`, `limits`, and the
//! `Path` / `Read` / `Write` / `Execute` keys inside each `PathAccess`,
//! plus the `MemoryBytes` / `CPUSeconds` / `MaxProcs` / `MaxFiles` keys
//! inside `Limits`) match Go's default JSON encoding exactly.

use std::io::{Read, Write};
use std::sync::Arc;

use serde::Serialize;

use crate::daemon::runtime::landlock::PathAccess;
use crate::daemon::runtime::rlimit::Limits;

/// Name of the wrapper subcommand. Matches
/// `execsandbox.GetSpec().Name` in the Go tree (kept on this phase so the
/// existing Go wrapper can consume the output until phase 7 deletes it).
pub const WRAPPER_SUBCOMMAND: &str = "_exec-sandbox";

/// Name of the env var that carries the JSON config blob to the wrapper.
/// Renamed from the dead-brand name the Go tree still carries in its
/// wrapper command, because the Rust tree is the post-rename namespace.
pub const CONFIG_ENV_VAR: &str = "UNITPM_SANDBOX_CONFIG";

/// Inputs to [`wrap_sandbox`]. Mirrors `SandboxOptions`.
#[derive(Debug, Clone, Default)]
pub struct SandboxOptions {
	/// Path to the `unitpm` binary that will be invoked as the wrapper.
	pub binary: String,
	/// Working directory for the wrapped process.
	pub cwd: String,
	/// Log directory passed through to the wrapper.
	pub log_dir: String,
	/// Resource caps applied by the wrapper.
	pub limits: Limits,
	/// Allow-list override; empty means "use the wrapper's defaults".
	pub allow: Vec<PathAccess>,
}

/// Description of the original (pre-wrap) command. Mirrors the fields the
/// Go `WrapSandbox` reads from `*exec.Cmd`: `Path`, `Args[1:]`, `Env`,
/// `Stdin`, `Stdout`, `Stderr`.
#[derive(Default)]
pub struct CommandLike {
	/// Path to the binary to run inside the sandbox.
	pub path: String,
	/// Arguments to pass to the binary (without the binary name).
	pub args: Vec<String>,
	/// Environment variables to propagate.
	pub env: Vec<String>,
	/// Optional stdin stream. The wrapper inherits the parent's stream
	/// rather than re-creating one, so the test can verify identity.
	pub stdin: Option<Arc<dyn Read + Send + Sync>>,
	/// Optional stdout stream.
	pub stdout: Option<Arc<dyn Write + Send + Sync>>,
	/// Optional stderr stream.
	pub stderr: Option<Arc<dyn Write + Send + Sync>>,
}

impl std::fmt::Debug for CommandLike {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.debug_struct("CommandLike")
			.field("path", &self.path)
			.field("args", &self.args)
			.field("env", &self.env)
			.field("stdin", &self.stdin.as_ref().map(|_| "<stream>"))
			.field("stdout", &self.stdout.as_ref().map(|_| "<stream>"))
			.field("stderr", &self.stderr.as_ref().map(|_| "<stream>"))
			.finish()
	}
}

/// Per-process syscall attributes. Mirrors the Linux-only fields of
/// `syscall.SysProcAttr` that this module touches.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SysProcAttr {
	/// Union of namespace flags passed to `clone(2)` via the wrapper.
	pub clone_flags: u32,
	/// `setgroups` is disabled inside the user namespace so the wrapper
	/// cannot be used as a privilege-escalation primitive. Matches the
	/// Go `GidMappingsEnableSetgroups: false`.
	pub gid_mappings_enable_setgroups: bool,
	/// `setpgid` — the wrapper becomes the leader of a new process
	/// group so the daemon can `kill(-pid)` the group on `Stop`.
	pub set_pgid: bool,
	/// UID mappings for the new user namespace. A single mapping from
	/// the current UID to namespace UID 0.
	pub uid_mappings: Vec<IdMapping>,
	/// GID mappings for the new user namespace.
	pub gid_mappings: Vec<IdMapping>,
}

/// UID/GID mapping entry. Mirrors `syscall.SysProcIDMap`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct IdMapping {
	/// UID/GID inside the new namespace.
	pub container_id: u32,
	/// UID/GID on the host.
	pub host_id: u32,
	/// Number of consecutive IDs the mapping covers.
	pub size: u32,
}

/// Result of [`wrap_sandbox`]. Mirrors the wrapped `*exec.Cmd` the Go
/// implementation returns.
///
/// `Debug` is implemented manually because the propagated stdio streams
/// are trait objects and would not be `Debug`-able otherwise.
pub struct WrappedCommand {
	/// Path to the wrapper binary.
	pub binary: String,
	/// Wrapper arguments: `[<binary>, WRAPPER_SUBCOMMAND]`.
	pub args: Vec<String>,
	/// Environment, extended with `CONFIG_ENV_VAR=<json>`.
	pub env: Vec<String>,
	/// Propagated stdin stream.
	pub stdin: Option<Arc<dyn Read + Send + Sync>>,
	/// Propagated stdout stream.
	pub stdout: Option<Arc<dyn Write + Send + Sync>>,
	/// Propagated stderr stream.
	pub stderr: Option<Arc<dyn Write + Send + Sync>>,
	/// Syscall attributes applied to the wrapper command.
	pub sys_proc_attr: SysProcAttr,
}

impl std::fmt::Debug for WrappedCommand {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.debug_struct("WrappedCommand")
			.field("binary", &self.binary)
			.field("args", &self.args)
			.field("env", &self.env)
			.field("stdin", &self.stdin.as_ref().map(|_| "<stream>"))
			.field("stdout", &self.stdout.as_ref().map(|_| "<stream>"))
			.field("stderr", &self.stderr.as_ref().map(|_| "<stream>"))
			.field("sys_proc_attr", &self.sys_proc_attr)
			.finish()
	}
}

/// Errors surfaced by [`wrap_sandbox`]. The Go implementation returns a
/// `*exec.Cmd` and an `error`; the Rust port returns `Result<_, _>` for
/// symmetry. The [`Display`](std::fmt::Display) impl preserves the same
/// wire-visible text the Go errors use.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum SandboxError {
	/// `opts.binary` was empty.
	BinaryNotSet,
	/// `serde_json` rejected the config payload we built. Should not be
	/// reachable in practice — the payload is a fixed-shape struct — but
	/// surfaces it so a future field added without a serde rename cannot
	/// silently corrupt the wire format.
	Serialize(String),
}

impl std::fmt::Display for SandboxError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			SandboxError::BinaryNotSet => f.write_str("sandbox: binary not set"),
			SandboxError::Serialize(e) => write!(f, "sandbox serialize: {e}"),
		}
	}
}

impl std::error::Error for SandboxError {}

/// Assemble the unprivileged sandbox wrapper command.
///
/// The flow mirrors `WrapSandbox`:
///   1. Validate `opts.binary`.
///   2. Probe `landlock::supported`; print a warning when unsupported —
///      this is a soft-fail path, not an error return. Landlock absence
///      leaves every other hardening step (rlimits, namespaces, no-new-
///      privs) intact, so the wrapper still produces a more-confined
///      child than the parent.
///   3. Build the `execsandbox.Config`-shaped JSON payload.
///   4. Construct the wrapper command with the JSON in
///      `UNITPM_SANDBOX_CONFIG`, the propagated stdio, and the new user +
///      PID + mount namespaces.
pub fn wrap_sandbox(
	cmd: &CommandLike,
	opts: &SandboxOptions,
) -> Result<WrappedCommand, SandboxError> {
	if opts.binary.is_empty() {
		return Err(SandboxError::BinaryNotSet);
	}

	if !crate::daemon::runtime::landlock::supported() {
		// Best-effort: continue without landlock but keep other primitives.
		// A future flag could force abort instead.
		eprintln!("unitpm: warning: kernel does not support Landlock; sandbox will be weaker");
	}

	let payload = serde_json::to_string(&SandboxConfigJson {
		cwd: &opts.cwd,
		log_dir: opt_str(&opts.log_dir),
		allow: if opts.allow.is_empty() {
			None
		} else {
			Some(&opts.allow)
		},
		limits: &opts.limits,
		command: &cmd.path,
		args: &cmd.args,
	})
	.map_err(|e| SandboxError::Serialize(e.to_string()))?;

	// Construct the wrapper command.
	let args = vec![opts.binary.clone(), WRAPPER_SUBCOMMAND.to_string()];

	// Propagate env plus the config blob. The Go implementation uses
	// `append(cmd.Env, ...)` which intentionally mutates neither slice
	// in place — the Rust port builds a fresh vector to match.
	let mut env = cmd.env.clone();
	env.push(format!("{CONFIG_ENV_VAR}={payload}"));

	let uid = current_uid();
	let gid = current_gid();
	let clone_flags = libc::CLONE_NEWUSER | libc::CLONE_NEWPID | libc::CLONE_NEWNS;

	Ok(WrappedCommand {
		binary: opts.binary.clone(),
		args,
		env,
		stdin: cmd.stdin.clone(),
		stdout: cmd.stdout.clone(),
		stderr: cmd.stderr.clone(),
		sys_proc_attr: SysProcAttr {
			clone_flags: clone_flags as u32,
			gid_mappings_enable_setgroups: false,
			set_pgid: true,
			uid_mappings: vec![IdMapping {
				container_id: 0,
				host_id: uid,
				size: 1,
			}],
			gid_mappings: vec![IdMapping {
				container_id: 0,
				host_id: gid,
				size: 1,
			}],
		},
	})
}

// ---- Helpers ---------------------------------------------------------------

fn opt_str(s: &str) -> Option<&str> {
	if s.is_empty() {
		None
	} else {
		Some(s)
	}
}

fn current_uid() -> u32 {
	unsafe { libc::geteuid() as u32 }
}

fn current_gid() -> u32 {
	unsafe { libc::getegid() as u32 }
}

// Silence dead-code for the imports kept for symmetry with the Go signature.
fn _assert_send<T: Send + Sync>() {}
fn _assert_send_sync() {
	_assert_send::<WrappedCommand>();
	_assert_send::<SysProcAttr>();
	_assert_send::<CommandLike>();
}

// ---- JSON wire format ------------------------------------------------------

/// JSON payload sent to the wrapper. Field names match the Go
/// `execsandbox.Config` struct tags exactly so the existing Go wrapper
/// can consume it.
#[derive(Serialize)]
struct SandboxConfigJson<'a> {
	#[serde(rename = "cwd")]
	cwd: &'a str,
	#[serde(rename = "log_dir", skip_serializing_if = "Option::is_none")]
	log_dir: Option<&'a str>,
	#[serde(rename = "allow", skip_serializing_if = "Option::is_none")]
	allow: Option<&'a Vec<PathAccess>>,
	#[serde(rename = "limits")]
	limits: &'a Limits,
	#[serde(rename = "command")]
	command: &'a str,
	#[serde(rename = "args")]
	args: &'a [String],
}
