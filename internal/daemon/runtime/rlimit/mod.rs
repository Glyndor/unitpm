//! `setrlimit(2)` wrapper for the sandbox runtime.
//!
//! Called from the child wrapper (the Go `_exec-sandbox` CLI subcommand; the
//! Rust equivalent lands on a later phase) before `execve`. Each limit in
//! [`Limits`] is a `u64` — the zero value means "do not set" and is the only
//! way to leave a cap alone. Soft and hard are set to the same value so the
//! child cannot raise them back.
//!
//! Mirrors `internal/daemon/runtime/rlimit/rlimit_linux.go`. The struct
//! shape, the order of `setrlimit` calls, and the error wrapping are kept
//! identical so that the test fixtures port one-to-one.

use libc::{self, c_uint};
use serde::Serialize;

/// Caps applied by the sandbox. A field of zero means "leave the cap alone".
///
/// Field names in the [`Serialize`](serde::Serialize) impl match the Go
/// `rlimit.Limits` JSON encoding (`MemoryBytes`, `CPUSeconds`, `MaxProcs`,
/// `MaxFiles`) so the wire payload the daemon writes is byte-compatible
/// with the existing Go wrapper until phase 7 deletes it.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize)]
pub struct Limits {
	/// Address-space cap (`RLIMIT_AS`). The process is killed with `SIGSEGV`
	/// when it exceeds this.
	#[serde(rename = "MemoryBytes")]
	pub memory_bytes: u64,
	/// CPU-time cap (`RLIMIT_CPU`). Process receives `SIGXCPU`.
	#[serde(rename = "CPUSeconds")]
	pub cpu_seconds: u64,
	/// Per-user process cap (`RLIMIT_NPROC`).
	#[serde(rename = "MaxProcs")]
	pub max_procs: u64,
	/// Per-process open-files cap (`RLIMIT_NOFILE`).
	#[serde(rename = "MaxFiles")]
	pub max_files: u64,
}

/// Apply each non-zero limit to the current process. Soft and hard are set
/// to the same value so the child cannot raise them back. Returns the first
/// failure with a prefix matching the Go implementation's error wrapping.
pub fn apply(limits: &Limits) -> Result<(), RlimitError> {
	if limits.memory_bytes > 0 {
		set_one(libc::RLIMIT_AS, limits.memory_bytes)
			.map_err(|e| RlimitError::As(e.to_string()))?;
	}
	if limits.cpu_seconds > 0 {
		set_one(libc::RLIMIT_CPU, limits.cpu_seconds)
			.map_err(|e| RlimitError::Cpu(e.to_string()))?;
	}
	if limits.max_procs > 0 {
		set_one(libc::RLIMIT_NPROC, limits.max_procs)
			.map_err(|e| RlimitError::Nproc(e.to_string()))?;
	}
	if limits.max_files > 0 {
		set_one(libc::RLIMIT_NOFILE, limits.max_files)
			.map_err(|e| RlimitError::Nofile(e.to_string()))?;
	}
	Ok(())
}

fn set_one(which: c_uint, value: u64) -> std::io::Result<()> {
	// SAFETY: `rlim_t` is a pair of `u64` values on Linux. The struct
	// mirrors the layout the kernel expects and is initialised before the
	// pointer is handed to the syscall.
	let rl = libc::rlimit {
		rlim_cur: value,
		rlim_max: value,
	};
	// SAFETY: the syscall takes a pointer to a kernel-layout `rlimit` and
	// cannot fail in a way that invalidates the value before the call.
	let r = unsafe { libc::setrlimit(which, &rl) };
	if r == 0 {
		Ok(())
	} else {
		Err(std::io::Error::last_os_error())
	}
}

/// Per-resource errors. Mirrors the Go "RLIMIT_<NAME>: %w" wrap.
#[derive(Debug)]
pub enum RlimitError {
	As(String),
	Cpu(String),
	Nproc(String),
	Nofile(String),
}

impl std::fmt::Display for RlimitError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			RlimitError::As(e) => write!(f, "RLIMIT_AS: {e}"),
			RlimitError::Cpu(e) => write!(f, "RLIMIT_CPU: {e}"),
			RlimitError::Nproc(e) => write!(f, "RLIMIT_NPROC: {e}"),
			RlimitError::Nofile(e) => write!(f, "RLIMIT_NOFILE: {e}"),
		}
	}
}

impl std::error::Error for RlimitError {}

#[cfg(test)]
mod tests;
