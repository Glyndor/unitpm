//! Helper functions used by [`manager`](super) and
//! [`process`](super::process). Lives in its own file so the registry
//! and process files stay under the 500-line gate.
//!
//! - [`ManagerError`] — the registry's error type, re-exported through
//!   the public surface.
//! - [`env_int`] — read an env var as a positive `i64`, falling back
//!   when missing or malformed.
//! - [`parse_simple_duration`] — minimal `humantime`-style parser for
//!   cron intervals (`5s`, `30m`, `24h`, `48h`).
//! - [`resolve_from_candidates`] — match the Go `resolveFromCandidates`
//!   that powers [`Manager::resolve_id`](super::Manager::resolve_id).
//! - [`rotate_loop`] / [`spawn_rotate_loop`] — the daemon-wide
//!   log-rotation goroutine and its launcher.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::{Duration, Instant};

#[derive(Debug)]
pub enum ManagerError {
	AlreadyExists(String),
	ProcessNotFound(String),
	Spawn(String),
	Resolve(String),
	/// `Restore()` failure.
	Restore(String),
	InvalidTarget,
	NoTemplate(String),
	Scale(String),
	Reload(String),
	Limits(String),
}

impl std::fmt::Display for ManagerError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			ManagerError::AlreadyExists(s) => write!(f, "process with ID {s} already exists"),
			ManagerError::ProcessNotFound(s) => write!(f, "process not found: {s}"),
			ManagerError::Spawn(s) => write!(f, "spawn: {s}"),
			ManagerError::Resolve(s) => write!(f, "resolve: {s}"),
			ManagerError::Restore(s) => write!(f, "failed to load specs: {s}"),
			ManagerError::InvalidTarget => {
				f.write_str("ERR_BAD_REQUEST: target count must be >= 0")
			}
			ManagerError::NoTemplate(s) => write!(
				f,
				"ERR_NOT_FOUND: no existing instance of {s:?} to use as template"
			),
			ManagerError::Scale(s) => write!(f, "scale: {s}"),
			ManagerError::Reload(s) => write!(f, "reload: {s}"),
			ManagerError::Limits(s) => write!(f, "ERR_LIMITS: {s}"),
		}
	}
}

impl std::error::Error for ManagerError {}

pub fn env_int(key: &str, fallback: i64) -> i64 {
	std::env::var(key)
		.ok()
		.and_then(|s| s.parse::<i64>().ok())
		.filter(|n| *n > 0)
		.unwrap_or(fallback)
}

pub fn parse_simple_duration(s: &str) -> Result<Duration, String> {
	let s = s.trim();
	if s.is_empty() {
		return Err("empty".into());
	}
	let (num, unit) = s.split_at(
		s.find(|c: char| !c.is_ascii_digit())
			.ok_or_else(|| "missing unit".to_string())?,
	);
	let n: u64 = num
		.parse()
		.map_err(|e: std::num::ParseIntError| e.to_string())?;
	let multiplier: u64 = match unit {
		"s" => 1,
		"m" => 60,
		"h" => 3600,
		"d" => 86_400,
		other => return Err(format!("unsupported unit {other:?}")),
	};
	Ok(Duration::from_secs(n * multiplier))
}

pub fn resolve_from_candidates(
	identifier: &str,
	candidates: &[String],
) -> Result<String, ManagerError> {
	match candidates.len() {
		0 => Err(ManagerError::ProcessNotFound(format!(
			"{identifier} (run 'unitpm list' to see all processes)"
		))),
		1 => Ok(candidates[0].clone()),
		_ => Err(ManagerError::ProcessNotFound(format!(
			"ambiguous selector '{identifier}': matches {} processes {:?}",
			candidates.len(),
			candidates
		))),
	}
}

pub fn rotate_loop(stop_flag: Arc<AtomicBool>) {
	let interval_ms = env_int("UNITPM_LOG_ROTATE_INTERVAL_MS", 60_000);
	if interval_ms <= 0 {
		return;
	}
	let mut next = Instant::now() + Duration::from_millis(interval_ms as u64);
	loop {
		if stop_flag.load(Ordering::Relaxed) {
			return;
		}
		let now = Instant::now();
		if now >= next {
			// Daemon-wide rotate would walk every process's writers; the
			// actual process list is shared state we don't carry here.
			// The ticker is the trigger for the per-process thread that
			// will follow; for now we just keep the ticker running.
			next = now + Duration::from_millis(interval_ms as u64);
			if env_int("UNITPM_TRIM_HEAP", 1) != 0 {
				// No GC hook in Rust; the trim was a Go-runtime detail.
			}
		}
		std::thread::sleep(Duration::from_millis(50));
	}
}

/// Spawn the daemon-wide rotation goroutine. Returns the stop flag and
/// the join handle so [`Manager::shutdown`](super::Manager::shutdown) can
/// join cleanly.
pub fn spawn_rotate_loop() -> (Arc<AtomicBool>, thread::JoinHandle<()>) {
	let stop_flag = Arc::new(AtomicBool::new(false));
	let flag_for_thread = stop_flag.clone();
	let handle = thread::spawn(move || {
		rotate_loop(flag_for_thread);
	});
	(stop_flag, handle)
}
