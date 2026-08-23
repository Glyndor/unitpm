//! Process-tree metrics collector.
//!
//! Reads parent/child relationships from `/proc/<pid>/stat` and aggregates
//! `utime + stime` (jiffies) plus `rss` (pages) across a process subtree.
//!
//! The global snapshot cache — a one-second-TTL map of parent->children —
//! lives behind a [`std::sync::Mutex`]. When N collectors run at the same
//! time (one per managed process), only the first scan walks /proc; the
//! rest share the snapshot for up to a second. The cache is process-global,
//! so the tests clear it via [`tests::clear_proc_tree_cache_for_tests`] and
//! bracket their bodies in [`tests::ProcTreeCacheGuard`] so a panic cannot
//! leak state into the next test.

use std::collections::HashMap;
use std::fs;
use std::sync::{Mutex, OnceLock};
use std::time::{Instant, SystemTime};

use super::{ChildStat, Collector, Metrics, MetricsError};

/// Default clock ticks per second on Linux. Most systems use 100.
const DEFAULT_CLK_TCK: f64 = 100.0;

/// Memory page size used to convert `rss` (pages) to bytes. Matches the
/// Go code which also hardcodes 4096 — `getconf PAGESIZE` on the test host
/// confirms this for every Linux we care about.
const PAGE_SIZE: i64 = 4096;

/// One PID's parent — present in the snapshot only when the parent's stat
/// file could be read.
type TreeSnapshot = HashMap<i32, Vec<i32>>;

static PROC_CACHE: OnceLock<Mutex<Option<(TreeSnapshot, Instant)>>> = OnceLock::new();

fn cache_cell() -> &'static Mutex<Option<(TreeSnapshot, Instant)>> {
	PROC_CACHE.get_or_init(|| Mutex::new(None))
}

fn take_snapshot() -> Result<TreeSnapshot, MetricsError> {
	let mut tree: TreeSnapshot = HashMap::new();
	for entry in fs::read_dir("/proc")? {
		let entry = entry?;
		let name = entry.file_name();
		let name_str = match name.to_str() {
			Some(s) => s,
			None => continue,
		};
		let pid: i32 = match name_str.parse() {
			Ok(p) => p,
			Err(_) => continue,
		};
		match get_ppid(pid) {
			Ok(ppid) => {
				tree.entry(ppid).or_default().push(pid);
			}
			Err(_) => continue,
		}
	}
	Ok(tree)
}

fn snapshot(force_refresh: bool) -> Result<TreeSnapshot, MetricsError> {
	let cell = cache_cell();
	let mut guard = cell.lock().expect("proc tree cache poisoned");
	let now = Instant::now();
	if !force_refresh {
		if let Some((tree, cached_at)) = guard.as_ref() {
			if now.duration_since(*cached_at) < std::time::Duration::from_secs(1) {
				return Ok(tree.clone());
			}
		}
	}
	let tree = take_snapshot()?;
	*guard = Some((tree.clone(), now));
	Ok(tree)
}

/// Per-PID parent PID. Reads `/proc/<pid>/stat` and uses the `comm`-field
/// parens trick (find the last `)`) so process names with spaces still
/// parse.
pub fn get_ppid(pid: i32) -> Result<i32, MetricsError> {
	let bytes = fs::read(format!("/proc/{pid}/stat"))?;
	let last_paren = bytes
		.iter()
		.rposition(|&b| b == b')')
		.ok_or(MetricsError::InvalidStatFormat)?;
	if last_paren + 2 >= bytes.len() {
		return Err(MetricsError::InvalidStatFormat);
	}
	let rest = &bytes[last_paren + 2..];
	let mut iter = rest.split(|b: &u8| b.is_ascii_whitespace());
	let _state = iter.next().ok_or(MetricsError::InvalidStatFormat)?;
	let ppid_bytes = iter.next().ok_or(MetricsError::InvalidStatFormat)?;
	let s = std::str::from_utf8(ppid_bytes)
		.map_err(|e| MetricsError::Io(std::io::Error::other(format!("invalid utf-8: {e}"))))?;
	s.parse::<i32>().map_err(MetricsError::from)
}

/// Aggregates `utime + stime` and `rss` across a process subtree.
#[derive(Debug)]
pub struct ProcTreeCollector {
	root_pid: i32,
	last_total_ticks: Option<i64>,
	last_sample_time: Option<SystemTime>,
}

impl ProcTreeCollector {
	/// Build a collector for `root_pid`. Errors if `/proc/<root_pid>` does
	/// not exist; the snapshot may still contain descendants whose stats
	/// vanish mid-scan.
	pub fn new(root_pid: i32) -> Result<Self, MetricsError> {
		let probe = format!("/proc/{root_pid}");
		match fs::metadata(&probe) {
			Ok(_) => Ok(Self {
				root_pid,
				last_total_ticks: None,
				last_sample_time: None,
			}),
			Err(e) => Err(MetricsError::Io(e)),
		}
	}

	/// PID the collector is rooted at.
	#[must_use]
	pub fn root_pid(&self) -> i32 {
		self.root_pid
	}

	fn read_proc_stat(&self, pid: i32) -> Result<(i64, i64), MetricsError> {
		let bytes = fs::read(format!("/proc/{pid}/stat"))?;
		let last_paren = bytes
			.iter()
			.rposition(|&b| b == b')')
			.ok_or(MetricsError::InvalidStatFormat)?;
		let rest = &bytes[last_paren + 2..];
		let mut iter = rest.split(|b: &u8| b.is_ascii_whitespace());
		// Skip past state (0), ppid (1), pgrp (2), session (3),
		// tty_nr (4), tpgid (5), flags (6), minflt (7), cminflt (8),
		// majflt (9), cmajflt (10). utime is index 11.
		for _ in 0..11 {
			iter.next().ok_or(MetricsError::InvalidStatFormat)?;
		}
		let utime = parse_field(&mut iter)?;
		let stime = parse_field(&mut iter)?;
		// Skip remaining fields up to rss. The index within the
		// post-comm slice is 21 (the 24th overall field).
		for _ in 0..8 {
			iter.next().ok_or(MetricsError::InvalidStatFormat)?;
		}
		let rss = parse_field(&mut iter)?;
		Ok((utime + stime, rss))
	}
}

impl Collector for ProcTreeCollector {
	fn kind(&self) -> &'static str {
		"proctree"
	}

	fn collect(&mut self) -> Result<Metrics, MetricsError> {
		let now = SystemTime::now();
		let mut m = Metrics {
			timestamp: now,
			memory_bytes: 0,
			cpu_percent: 0.0,
		};

		let pids = match find_descendants(self.root_pid) {
			Ok(p) => p,
			Err(_) => vec![self.root_pid],
		};

		let mut total_ticks: i64 = 0;
		let mut total_rss: i64 = 0;
		for pid in pids {
			if let Ok((ticks, rss_pages)) = self.read_proc_stat(pid) {
				total_ticks += ticks;
				total_rss += rss_pages;
			}
		}
		m.memory_bytes = total_rss * PAGE_SIZE;

		if let (Some(prev_ticks), Some(prev_time)) = (self.last_total_ticks, self.last_sample_time)
		{
			let delta_ticks = total_ticks - prev_ticks;
			let delta_secs = now
				.duration_since(prev_time)
				.map(|d| d.as_secs_f64())
				.unwrap_or(0.0);
			if delta_secs > 0.0 && delta_ticks >= 0 {
				m.cpu_percent = (delta_ticks as f64 / DEFAULT_CLK_TCK) / delta_secs * 100.0;
			}
		}

		self.last_total_ticks = Some(total_ticks);
		self.last_sample_time = Some(now);

		Ok(m)
	}
}

fn parse_field<'a, I: Iterator<Item = &'a [u8]>>(iter: &mut I) -> Result<i64, MetricsError> {
	let bytes = iter.next().ok_or(MetricsError::InvalidStatFormat)?;
	let s = std::str::from_utf8(bytes)
		.map_err(|e| MetricsError::Io(std::io::Error::other(format!("invalid utf-8: {e}"))))?;
	s.parse::<i64>().map_err(MetricsError::from)
}

/// Walk the cached snapshot BFS, starting at `root`, and return every
/// descendant plus the root itself.
fn find_descendants(root: i32) -> Result<Vec<i32>, MetricsError> {
	let tree = snapshot(false)?;
	let mut out = vec![root];
	let mut queue = vec![root];
	while let Some(cur) = queue.first().copied() {
		queue.remove(0);
		if let Some(children) = tree.get(&cur) {
			out.extend_from_slice(children);
			queue.extend_from_slice(children);
		}
	}
	Ok(out)
}

/// Return a depth-first ordered slice of [`ChildStat`] entries for the
/// process subtree rooted at `root_pid`. Processes that disappear between
/// the snapshot and the read are silently skipped.
pub fn get_process_tree(root_pid: i32) -> Result<Vec<ChildStat>, MetricsError> {
	let tree = snapshot(false)?;
	let mut out = Vec::new();
	let mut queue: Vec<(i32, i32)> = vec![(root_pid, 0)];
	while let Some((cur, depth)) = queue.first().copied() {
		queue.remove(0);

		let rss = match read_stat_rss(cur) {
			Ok(r) => r,
			Err(_) => continue,
		};

		out.push(ChildStat {
			pid: cur,
			comm: read_comm(cur),
			depth,
			memory_bytes: rss * PAGE_SIZE,
		});

		if let Some(children) = tree.get(&cur) {
			for child in children {
				queue.push((*child, depth + 1));
			}
		}
	}
	Ok(out)
}

fn read_stat_rss(pid: i32) -> Result<i64, MetricsError> {
	let bytes = fs::read(format!("/proc/{pid}/stat"))?;
	let last_paren = bytes
		.iter()
		.rposition(|&b| b == b')')
		.ok_or(MetricsError::InvalidStatFormat)?;
	let rest = &bytes[last_paren + 2..];
	let mut iter = rest.split(|b: &u8| b.is_ascii_whitespace());
	for _ in 0..21 {
		iter.next().ok_or(MetricsError::InvalidStatFormat)?;
	}
	parse_field(&mut iter)
}

fn read_comm(pid: i32) -> String {
	let path = format!("/proc/{pid}/comm");
	match fs::read_to_string(&path) {
		Ok(s) => s.trim().to_string(),
		Err(_) => String::new(),
	}
}

/// Convenience constructor matching the Go `NewProcTreeCollector`.
pub fn new_proc_tree_collector(pid: i32) -> Result<ProcTreeCollector, MetricsError> {
	ProcTreeCollector::new(pid)
}

/// Test-only helpers. Exposed via the crate root under
/// `cfg(all(test, target_os = "linux"))`.
#[cfg(all(test, target_os = "linux"))]
pub mod tests {
	use super::*;

	/// Drop guard that snapshots the proc-tree cache on construction and
	/// restores it on `Drop`. Does **not** hold the cache lock for the
	/// duration of the test body — the metrics code path itself needs to
	/// take the same lock to read /snapshot, so holding it across the
	/// body would self-deadlock on a single thread. The race window
	/// between `new()` and `Drop` is bounded by the test runtime; tests
	/// that need strict serialisation around the cache use [`PROC_LOCK`]
	/// (in `tests.rs`) in addition to this guard.
	///
	/// `PROC_LOCK` is the global test serialisation mutex defined in
	/// `tests.rs`; this struct does not depend on it.
	pub struct ProcTreeCacheGuard {
		saved: Option<(TreeSnapshot, Instant)>,
	}

	impl ProcTreeCacheGuard {
		#[must_use]
		pub fn new() -> Self {
			let cell = cache_cell();
			let saved = cell.lock().expect("proc tree cache poisoned").take();
			// `saved` is now `None` in the cell. The test body runs
			// against an empty cache; whichever cache writer populates
			// the cell next will get cached normally.
			Self { saved }
		}
	}

	impl Default for ProcTreeCacheGuard {
		fn default() -> Self {
			Self::new()
		}
	}

	impl Drop for ProcTreeCacheGuard {
		fn drop(&mut self) {
			let cell = cache_cell();
			let mut lock = cell.lock().expect("proc tree cache poisoned");
			// Only restore if the test body didn't already leave the
			// cell populated. This keeps tests that legitimately want
			// to seed the cache from getting clobbered.
			if lock.is_none() {
				*lock = self.saved.take();
			}
		}
	}

	/// Drop the cached snapshot unconditionally. Useful when a test wants
	/// the next read to walk /proc afresh without saving the prior value.
	pub fn clear_proc_tree_cache_for_tests() {
		let cell = cache_cell();
		let mut lock = cell.lock().expect("proc tree cache poisoned");
		*lock = None;
	}

	/// Force the next snapshot to be a fresh /proc walk, regardless of the
	/// cache TTL.
	#[allow(dead_code)]
	pub fn force_refresh() {
		let _ = super::snapshot(true);
	}
}
