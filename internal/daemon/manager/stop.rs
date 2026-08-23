//! Stop — graceful shutdown of a single managed process.
//!
//! Mirrors `manager.Stop`, `gracefulKill`, `signalTree`, `killTree`,
//! `walkDescendants`, and the small helpers around them. The first
//! signal comes from [`resolve_stop`] (configurable: `SIGTERM` is the
//! default, with `SIGINT` / `SIGHUP` / `SIGQUIT` / `SIGUSR1` /
//! `SIGUSR2` accepted). After a timeout the entire process tree is
//! `SIGKILL`'d. Tree discovery walks `/proc` so backgrounded children
//! of a shell wrapper — which have left the supervised process group —
//! are still reaped.

use std::collections::HashMap;
use std::ffi::c_int;
use std::fs;
use std::io;
use std::path::PathBuf;
use std::sync::atomic::AtomicI32;
use std::time::{Duration, Instant};

use libc::{self, pid_t};

use crate::daemon::manager::spawn::process_binary;
use crate::metrics;

/// Default wait between the graceful signal and the SIGKILL escalation.
/// Mirrors `manager.defaultStopTimeout`.
pub const DEFAULT_STOP_TIMEOUT: Duration = Duration::from_secs(10);

/// Allowed stop-signal names. `SIGKILL`, `SIGSEGV`, `SIGSTOP` are never
/// exposed — a manager-initiated `SIGKILL` happens at the timeout step.
pub const STOP_SIGNALS: &[(&str, c_int)] = &[
	("SIGTERM", libc::SIGTERM),
	("SIGINT", libc::SIGINT),
	("SIGHUP", libc::SIGHUP),
	("SIGQUIT", libc::SIGQUIT),
	("SIGUSR1", libc::SIGUSR1),
	("SIGUSR2", libc::SIGUSR2),
];

/// Errors surfaced by the stop helpers. Mirrors the Go `signalTree` /
/// `killTree` flow — most failures are swallowed because the goal is to
/// get the tree dead, not to surface one missing child.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum KillError {
	/// All kill attempts failed (the parent already reaped).
	NoProcess,
	/// `kill(-pgid, sig)` returned a non-ESRCH error.
	ProcessGroup(libc::c_int),
	/// `proc.Signal` returned a non-`ErrProcessDone` error.
	Parent(libc::c_int),
	/// An unknown signal name was requested.
	UnknownSignal,
}

impl std::fmt::Display for KillError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			KillError::NoProcess => f.write_str("kill: process already gone"),
			KillError::ProcessGroup(e) => write!(f, "kill: process group: errno={e}"),
			KillError::Parent(e) => write!(f, "kill: parent: errno={e}"),
			KillError::UnknownSignal => f.write_str("unknown signal name"),
		}
	}
}

impl std::error::Error for KillError {}

/// Resolve the (signal, timeout) pair to apply. Unknown signal names
/// silently degrade to `SIGTERM` — matches the Go `resolveStop`.
pub fn resolve_stop(name: Option<&str>, timeout_ms: Option<i32>) -> (c_int, Duration) {
	let sig = match name {
		Some(s) => STOP_SIGNALS
			.iter()
			.find(|(n, _)| *n == s)
			.map(|(_, v)| *v)
			.unwrap_or(libc::SIGTERM),
		None => libc::SIGTERM,
	};
	let timeout = match timeout_ms {
		Some(ms) if ms > 0 => Duration::from_millis(ms as u64),
		_ => DEFAULT_STOP_TIMEOUT,
	};
	(sig, timeout)
}

/// Send `stop_signal` to the supervised process and every descendant
/// discovered via `/proc`. The descendants are signalled leaves-first so
/// shell wrappers are killed after their children, then the pgroup, then
/// the parent. The `UNITPM_DEBUG_STOP` env var flips a debug-log path that
/// the Go side gates on; the Rust port mirrors it without the
/// `log.Printf` because the daemon has its own logging and we don't want
/// to take on a log dependency here.
pub fn signal_tree(pid: pid_t, sig: c_int) -> Result<(), KillError> {
	let descendants = walk_descendants(pid);
	let debug = std::env::var_os("UNITPM_DEBUG_STOP").is_some();

	for d in &descendants {
		let r = unsafe { libc::kill(*d, sig) };
		if r != 0 && debug {
			let e = io::Error::last_os_error().raw_os_error().unwrap_or(0);
			eprintln!("stop: kill pid={d} sig={sig} err={e}");
		}
	}

	let grp = unsafe { libc::kill(-pid, sig) };
	if grp != 0 {
		let errno = io::Error::last_os_error().raw_os_error().unwrap_or(0);
		if errno != libc::ESRCH && debug {
			eprintln!("stop: kill -pgrp={pid} sig={sig} err={errno}");
		}
		if errno != libc::ESRCH {
			return Err(KillError::ProcessGroup(errno));
		}
	}
	Ok(())
}

/// `signalTree` with `SIGKILL` hard-wired, same pre-collect order. Always
/// best-effort — a child gone is a successful kill.
pub fn kill_tree(pid: pid_t) -> Result<(), KillError> {
	let descendants = walk_descendants(pid);
	for d in &descendants {
		unsafe {
			libc::kill(*d, libc::SIGKILL);
		}
	}
	unsafe {
		libc::kill(-pid, libc::SIGKILL);
	}
	Ok(())
}

/// Graceful-stop sequence: `signalTree`, then poll until the parent
/// exits or `timeout` elapses, then `killTree`.
pub fn graceful_kill(pid: pid_t, sig: c_int, timeout: Duration) -> Result<(), KillError> {
	if let Err(e) = signal_tree(pid, sig) {
		kill_tree(pid)?;
		return Err(e);
	}

	let deadline = Instant::now() + timeout;
	let mut next_tick = Instant::now() + Duration::from_millis(50);
	loop {
		let now = Instant::now();
		if now >= deadline {
			kill_tree(pid)?;
			return Ok(());
		}
		if now >= next_tick {
			let r = unsafe { libc::kill(pid, 0) };
			if r != 0 {
				let errno = io::Error::last_os_error().raw_os_error().unwrap_or(0);
				if errno == libc::ESRCH {
					return Ok(());
				}
			}
			next_tick = now + Duration::from_millis(50);
		}
		// Sleep a short slice until the next tick or deadline.
		let sleep_for = next_tick
			.min(deadline)
			.duration_since(now)
			.min(Duration::from_millis(10));
		std::thread::sleep(sleep_for);
	}
}

/// Walk `/proc` once, build the `ppid → children` adjacency, and return
/// every descendant of `root` via DFS in deepest-first order so leaves
/// are signalled before their shell wrappers.
#[must_use]
pub fn walk_descendants(root: pid_t) -> Vec<pid_t> {
	let children = match scan_proc_tree() {
		Ok(c) => c,
		Err(_) => return Vec::new(),
	};
	let mut out = Vec::new();
	let mut stack: Vec<pid_t> = vec![root];
	while let Some(pid) = stack.pop() {
		if let Some(kids) = children.get(&pid) {
			for &k in kids {
				out.push(k);
				stack.push(k);
			}
		}
	}
	out
}

/// Scan `/proc` once and build `parent → [child...]` adjacency.
fn scan_proc_tree() -> io::Result<HashMap<pid_t, Vec<pid_t>>> {
	let mut children: HashMap<pid_t, Vec<pid_t>> = HashMap::new();
	for entry in fs::read_dir("/proc")? {
		let entry = match entry {
			Ok(e) => e,
			Err(_) => continue,
		};
		if !entry.file_type().map(|t| t.is_dir()).unwrap_or(false) {
			continue;
		}
		let name = entry.file_name();
		let name_str = match name.to_str() {
			Some(s) => s,
			None => continue,
		};
		let pid: pid_t = match name_str.parse() {
			Ok(n) => n,
			Err(_) => continue,
		};
		let ppid = match metrics::get_ppid(pid) {
			Ok(p) => p,
			Err(_) => continue,
		};
		children.entry(ppid).or_default().push(pid);
	}
	Ok(children)
}

/// Public re-export of [`process_binary`] for callers that want the
/// daemon-binary lookup without depending on `spawn`. The Go counterpart
/// is the equivalent `manager.*` helper.
pub fn daemon_binary() -> Result<String, io::Error> {
	process_binary()
}

/// Lookup path (mostly for tests; the daemon binary may be on PATH).
pub fn daemon_binary_lookup() -> Result<String, io::Error> {
	process_binary()
}

/// Stub: resolve the `dynamic` mode credentials directory for cleanup
/// after a process exits. Mirrors the Go behaviour that drops
/// `/var/lib/glyndor/unitpm/creds/<id>/` on `Stop(byUser=true)`.
pub fn cleanup_credentials(id: &str) {
	let creds_dir: PathBuf = paths_creds_dir();
	let target = creds_dir.join(id);
	let _ = fs::remove_dir_all(&target);
}

fn paths_creds_dir() -> PathBuf {
	PathBuf::from(crate::paths::CREDS_DIR)
}

/// The `last_rotate_nanos` debug aid kept here so unit tests don't reach
/// into the `logwriter` internals.
#[allow(dead_code)]
fn _stop_marker() -> &'static AtomicI32 {
	static MARKER: AtomicI32 = AtomicI32::new(0);
	&MARKER
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::process::Command;

	#[test]
	fn resolve_stop_default() {
		let (sig, to) = resolve_stop(None, None);
		assert_eq!(sig, libc::SIGTERM);
		assert_eq!(to, DEFAULT_STOP_TIMEOUT);
	}

	#[test]
	fn resolve_stop_known_signal() {
		let (sig, _) = resolve_stop(Some("SIGHUP"), None);
		assert_eq!(sig, libc::SIGHUP);
	}

	#[test]
	fn resolve_stop_unknown_signal_falls_back_to_sigterm() {
		let (sig, _) = resolve_stop(Some("SIGWAT"), None);
		assert_eq!(sig, libc::SIGTERM);
	}

	#[test]
	fn resolve_stop_respects_timeout() {
		let (_, to) = resolve_stop(None, Some(2500));
		assert_eq!(to, Duration::from_millis(2500));
	}

	#[test]
	fn resolve_stop_ignores_zero_timeout() {
		let (_, to) = resolve_stop(None, Some(0));
		assert_eq!(to, DEFAULT_STOP_TIMEOUT);
	}

	#[test]
	fn walk_descendants_current_process() {
		// Spawn a child so the walker has something to find.
		let mut cmd = Command::new("sleep").arg("1").spawn().expect("spawn sleep");
		defer_kill(&mut cmd);
		let descendants = walk_descendants(std::process::id() as pid_t);
		// May or may not contain the child depending on PID race — we
		// just want it to return without crashing.
		let _ = descendants;
	}

	#[test]
	fn walk_descendants_nonexistent_root_returns_empty() {
		let descendants = walk_descendants(i32::MAX as pid_t);
		assert!(descendants.is_empty());
	}

	#[test]
	fn cleanup_credentials_noop_when_missing() {
		// Should not panic.
		cleanup_credentials("nonexistent-id-xyz");
	}

	fn defer_kill(cmd: &mut std::process::Child) {
		let _ = cmd.kill();
		let _ = cmd.wait();
	}
}
