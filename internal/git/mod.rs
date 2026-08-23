//! Probe a repository's HEAD to surface branch / commit / dirty state in
//! process listings.
//!
//! The repository path is **user-supplied**: callers pass the cwd of a
//! managed process and expect the probe to fail gracefully on anything
//! that is not a working repository. The module shells out to `git` with
//! a 2-second timeout per call. Three places this can go wrong:
//!
//! 1. `git` is not on `PATH` → return an empty [`Info`], not an error.
//! 2. `<dir>/.git` is missing (not a repo) → return an empty [`Info`].
//! 3. `git` returns a non-zero exit (corrupt `.git`, broken HEAD,
//!    enormous repo) or runs past the 2-second timeout → leave the
//!    corresponding field empty; do not fail the whole probe. A failed
//!    probe MUST NOT bubble up to the caller, because listing processes
//!    must not break just because one of them sits in a directory with a
//!    corrupt `.git`.
//!
//! Subprocesses inherit the environment and the working directory from
//! the parent. We override `current_dir` per call (Go's `cmd.Dir`) so the
//! probe runs against the user-supplied path, but leave the env alone:
//! git uses the user's `GIT_CONFIG_*`, `GIT_AUTHOR_*`, and friends, and
//! that is correct. Tests should not assume the parent CWD is preserved
//! or modified — `get_info` only sets the subprocess CWD, never the
//! test process's.

use std::io::Read;
use std::path::Path;
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};

/// Maximum time a single `git` invocation may run before it is killed.
/// Mirrors the Go `context.WithTimeout(2 * time.Second)` on each
/// `exec.CommandContext` call.
pub const PROBE_TIMEOUT: Duration = Duration::from_secs(2);

/// Probe result. All three fields default to empty / false when the
/// target path is not a repository or `git` is unavailable.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Info {
	pub branch: String,
	pub commit: String,
	pub dirty: bool,
}

/// Probe `dir` for HEAD info. Never errors on a missing repository or a
/// broken `git` invocation — listing processes must not fail because one
/// of them happens to be in a corrupt `.git`.
#[must_use]
pub fn get_info(dir: &str) -> Info {
	let mut info = Info::default();

	if !git_available() {
		return info;
	}

	let dot_git = Path::new(dir).join(".git");
	if !dot_git.exists() {
		return info;
	}

	if let Some(branch) = run_git(dir, &["symbolic-ref", "--short", "HEAD"]) {
		info.branch = branch;
	} else {
		// Detached HEAD — Go returns the literal "detached".
		info.branch = "detached".to_string();
	}

	if let Some(commit) = run_git(dir, &["rev-parse", "--short", "HEAD"]) {
		info.commit = commit;
	}

	if check_dirty(dir) {
		info.dirty = true;
	}

	info
}

fn git_available() -> bool {
	Command::new("git")
		.arg("--version")
		.stdout(Stdio::null())
		.stderr(Stdio::null())
		.status()
		.map(|s| s.success())
		.unwrap_or(false)
}

fn run_git(dir: &str, args: &[&str]) -> Option<String> {
	let mut child = Command::new("git")
		.args(args)
		.current_dir(dir)
		.stdout(Stdio::piped())
		.stderr(Stdio::null())
		.spawn()
		.ok()?;

	let start = Instant::now();
	let result = loop {
		match child.try_wait().ok()? {
			Some(status) => {
				if !status.success() {
					break None;
				}
				let mut out = Vec::new();
				if let Some(mut stdout) = child.stdout.take() {
					let _ = stdout.read_to_end(&mut out);
				}
				let s = std::str::from_utf8(&out).ok()?;
				let trimmed = s.trim();
				break if trimmed.is_empty() {
					None
				} else {
					Some(trimmed.to_string())
				};
			}
			None => {
				if start.elapsed() >= PROBE_TIMEOUT {
					let _ = child.kill();
					let _ = child.wait();
					break None;
				}
				std::thread::sleep(Duration::from_millis(10));
			}
		}
	};
	result
}

fn check_dirty(dir: &str) -> bool {
	dirty_via(dir, &["diff", "--quiet"]) || dirty_via(dir, &["diff", "--cached", "--quiet"])
}

/// Returns true when `git diff [--cached] --quiet` exits 1, indicating
/// changes. Exit codes other than 0 or 1 (e.g. process killed by the
/// 2-second timeout) are treated as "unknown, assume clean" — never as
/// "dirty". That's the conservative choice: a probe that timed out
/// should not label a clean repo dirty.
fn dirty_via(dir: &str, args: &[&str]) -> bool {
	let mut child = match Command::new("git")
		.args(args)
		.current_dir(dir)
		.stdout(Stdio::null())
		.stderr(Stdio::null())
		.spawn()
	{
		Ok(c) => c,
		Err(_) => return false,
	};

	let start = Instant::now();
	let exit = loop {
		match child.try_wait() {
			Ok(Some(s)) => break Some(s),
			Ok(None) => {
				if start.elapsed() >= PROBE_TIMEOUT {
					let _ = child.kill();
					let _ = child.wait();
					break None;
				}
				std::thread::sleep(Duration::from_millis(10));
			}
			Err(_) => {
				let _ = child.kill();
				let _ = child.wait();
				break None;
			}
		}
	};
	match exit {
		Some(s) => matches!(s.code(), Some(1)),
		None => false,
	}
}

#[cfg(test)]
mod tests;
