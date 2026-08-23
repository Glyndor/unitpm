//! Recursive file watcher for `--watch`-style auto-restart.
//!
//! Mirrors `internal/daemon/manager/watcher.go`. Walks the directory tree
//! under `root`, records `(modtime, size)` per regular file, and emits the
//! `onChange` callback whenever the snapshot differs. Symlinks are
//! skipped — a planted symlink under the watch root would otherwise let
//! an attacker redirect the change event at an unrelated file.
//!
//! The watcher runs entirely off [`std::thread`] + a `Condvar`-less
//! channel-style `AtomicBool` cancel flag. The Go implementation uses a
//! `context.CancelFunc` for the same purpose; in Rust a one-shot `AtomicBool`
//! is sufficient because the ticker polls the flag on every wakeup and
//! exits cleanly when it sees the flip.

use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

const DEFAULT_INTERVAL: Duration = Duration::from_secs(2);
const MAX_WATCH_FILES: usize = 50_000;

/// One-time file metadata captured by the scanner.
#[derive(Debug, Clone, Copy)]
struct FileEntry {
	mod_time: std::time::SystemTime,
	size: u64,
}

/// Recursive directory watcher. Created via [`file_watcher`]; spawned by
/// [`FileWatcher::start`].
pub struct FileWatcher {
	root: PathBuf,
	ignore: Vec<String>,
	interval: Duration,
	on_change: Arc<dyn Fn() + Send + Sync + 'static>,

	mu: Mutex<WatcherState>,
}

#[derive(Default)]
struct WatcherState {
	cancel: Option<Arc<AtomicBool>>,
	running: bool,
	/// Last observed snapshot — owned by this watcher so concurrent
	/// watchers don't poison each other's diff.
	snapshot: HashMap<String, FileEntry>,
}

/// Build a watcher rooted at `root`. The `on_change` callback fires whenever
/// the file set under `root` differs from the previous snapshot.
#[must_use]
pub fn file_watcher(
	root: PathBuf,
	ignore: Vec<String>,
	on_change: Arc<dyn Fn() + Send + Sync + 'static>,
) -> FileWatcher {
	FileWatcher {
		root,
		ignore,
		interval: DEFAULT_INTERVAL,
		on_change,
		mu: Mutex::new(WatcherState::default()),
	}
}

impl FileWatcher {
	/// Override the polling cadence. Mirrors the Go test that pokes
	/// `w.interval = 100ms`.
	pub fn set_interval(&mut self, interval: Duration) {
		self.interval = interval;
	}

	/// Start the watcher goroutine. Idempotent: a second call is a no-op.
	pub fn start(&self) {
		let mut guard = self.mu.lock().unwrap_or_else(|e| e.into_inner());
		if guard.running {
			return;
		}
		guard.running = true;
		let initial = scan(&self.root, &self.ignore);
		guard.snapshot = initial;
		let cancel = Arc::new(AtomicBool::new(false));
		guard.cancel = Some(cancel.clone());

		let root = self.root.clone();
		let ignore = self.ignore.clone();
		let interval = self.interval;
		let cb = self.on_change.clone();
		let cancel_for_thread = cancel.clone();
		let parent_mu = Arc::new(Mutex::new(guard.snapshot.clone()));

		thread::spawn(move || {
			let mut next_at = Instant::now() + interval;
			loop {
				if cancel_for_thread.load(Ordering::Relaxed) {
					return;
				}
				let now = Instant::now();
				let sleep = next_at.saturating_duration_since(now);
				if !sleep.is_zero() {
					thread::sleep(sleep);
				}
				next_at += interval;
				let current = scan(&root, &ignore);
				if cancel_for_thread.load(Ordering::Relaxed) {
					return;
				}
				let last = parent_mu.lock().unwrap_or_else(|e| e.into_inner()).clone();
				let changed = diff_snapshots(&last, &current);
				if changed {
					cb();
				}
				*parent_mu.lock().unwrap_or_else(|e| e.into_inner()) = current;
			}
		});
	}

	/// Cancel the watcher goroutine. Safe to call before [`start`].
	pub fn stop(&self) {
		let mut guard = self.mu.lock().unwrap_or_else(|e| e.into_inner());
		if let Some(c) = guard.cancel.take() {
			c.store(true, Ordering::Relaxed);
		}
		guard.running = false;
		guard.snapshot.clear();
	}
}

/// Scan the tree under `root`, recording `(modtime, size)` per file.
fn scan(root: &Path, ignore: &[String]) -> HashMap<String, FileEntry> {
	let mut entries: HashMap<String, FileEntry> = HashMap::new();
	let root_len = root.to_string_lossy().len() + 1;
	let walker = walkdir_follow(root, ignore);
	for (count, (rel, meta)) in walker.into_iter().enumerate() {
		if count >= MAX_WATCH_FILES {
			break;
		}
		// Use `SystemTime` directly. It has well-defined equality across
		// scans of the same file (assuming no clock skew between the two
		// reads), unlike `Instant` which we can't reliably round-trip from
		// the file's mtime.
		let mod_time = meta.modified().unwrap_or(SystemTime::UNIX_EPOCH);
		entries.insert(
			rel,
			FileEntry {
				mod_time,
				size: meta.len(),
			},
		);
	}
	let _ = root_len;
	entries
}

/// Compare two snapshots. Returns `true` when either the size of any
/// file differs, OR when the mtime of any file differs. mtime is exact
/// `SystemTime` so two scans of an unchanged file see the same mtime
/// and don't fire — and a write genuinely changes the mtime.
fn diff_snapshots(old: &HashMap<String, FileEntry>, cur: &HashMap<String, FileEntry>) -> bool {
	if old.len() != cur.len() {
		return true;
	}
	for (path, ce) in cur {
		match old.get(path) {
			Some(oe) if oe.size == ce.size && oe.mod_time == ce.mod_time => {}
			_ => return true,
		}
	}
	false
}

/// Match `name` (basename) and `rel` (relative path) against `pattern`.
///
/// Mirrors `matchIgnore`:
/// - Rejects `..` and absolute paths (path-traversal hardening).
/// - Supports `*.ext` glob via `name.endswith(pattern[1:])`.
/// - Supports exact-name match.
/// - Falls back to `filepath.Match` against the relative path.
fn match_ignore(name: &str, rel: &str, pattern: &str) -> bool {
	if pattern.contains("..") || Path::new(pattern).is_absolute() {
		return false;
	}
	if let Some(rest) = pattern.strip_prefix("*.") {
		return name.ends_with(rest);
	}
	if name == pattern {
		return true;
	}
	Path::new(rel)
		.file_name()
		.map(|f| f == name)
		.unwrap_or(false)
		|| path_match(rel, pattern)
}

/// Tiny glob match supporting `*` (and only `*`). Replaces `filepath.Match`
/// for the limited pattern set the daemon cares about.
fn path_match(path: &str, pattern: &str) -> bool {
	let ps: Vec<&str> = pattern.split('*').collect();
	if ps.len() == 1 {
		return path == pattern;
	}
	let mut cursor = 0usize;
	for (i, chunk) in ps.iter().enumerate() {
		if chunk.is_empty() {
			continue;
		}
		match path[cursor..].find(chunk) {
			Some(idx) if i == 0 || idx == 0 => cursor += idx + chunk.len(),
			Some(idx) => cursor += idx + chunk.len(),
			None => return false,
		}
	}
	true
}

/// Walk `root` recursively, yielding `(relative_path, metadata)` for every
/// regular file. Symlinks are skipped.
fn walkdir_follow(root: &Path, ignore: &[String]) -> Vec<(String, fs::Metadata)> {
	let mut out = Vec::new();
	let mut stack = vec![root.to_path_buf()];
	while let Some(dir) = stack.pop() {
		let entries = match fs::read_dir(&dir) {
			Ok(e) => e,
			Err(_) => continue,
		};
		for entry in entries.flatten() {
			let ft = match entry.file_type() {
				Ok(t) => t,
				Err(_) => continue,
			};
			let name = entry.file_name();
			let name_str = name.to_string_lossy().to_string();
			let path = entry.path();
			let rel = path
				.strip_prefix(root)
				.map(|p| p.to_string_lossy().to_string())
				.unwrap_or_else(|_| name_str.clone());
			if ft.is_symlink() {
				if ft.is_dir() {
					continue;
				}
				continue;
			}
			if ft.is_dir() {
				if ignore.iter().any(|p| match_ignore(&name_str, &rel, p)) {
					continue;
				}
				stack.push(path);
				continue;
			}
			if !ft.is_file() {
				continue;
			}
			if ignore.iter().any(|p| match_ignore(&name_str, &rel, p)) {
				continue;
			}
			let meta = match entry.metadata() {
				Ok(m) => m,
				Err(_) => continue,
			};
			out.push((rel, meta));
		}
	}
	out
}

mod snapshot_store {
	use std::collections::HashMap;
	use std::path::{Path, PathBuf};
	use std::sync::Mutex;

	use super::FileEntry;

	static STORE: Mutex<Option<(PathBuf, HashMap<String, FileEntry>)>> = Mutex::new(None);

	pub fn last(root: &Path) -> HashMap<String, FileEntry> {
		let guard = STORE.lock().unwrap_or_else(|e| e.into_inner());
		match &*guard {
			Some((r, m)) if r == root => m.clone(),
			_ => HashMap::new(),
		}
	}

	pub fn set(root: PathBuf, snapshot: HashMap<String, FileEntry>) {
		let mut guard = STORE.lock().unwrap_or_else(|e| e.into_inner());
		*guard = Some((root, snapshot));
	}

	#[cfg(test)]
	pub fn clear() {
		let mut guard = STORE.lock().unwrap_or_else(|e| e.into_inner());
		*guard = None;
	}
}

fn last_snapshot(root: &Path, _ignore: &[String]) -> HashMap<String, FileEntry> {
	snapshot_store::last(root)
}

fn set_last_snapshot(root: &Path, _ignore: &[String], snapshot: HashMap<String, FileEntry>) {
	snapshot_store::set(root.to_path_buf(), snapshot);
}

/// Helper: zero `SystemTime` for the watcher's "never modified" sentinel.
trait OrZero {
	fn now_or_zero(self) -> SystemTime;
}

impl OrZero for SystemTime {
	fn now_or_zero(self) -> SystemTime {
		self
	}
}

use std::time::SystemTime;

#[cfg(test)]
mod tests {
	use super::*;
	use std::sync::atomic::AtomicI32;

	#[test]
	fn detects_change() {
		let dir = tempfile::tempdir().unwrap();
		let file = dir.path().join("test.txt");
		std::fs::write(&file, b"initial").unwrap();
		// Reset the global snapshot store so prior tests don't poison us.
		snapshot_store::clear();
		let called = Arc::new(AtomicI32::new(0));
		let cb = {
			let called = called.clone();
			Arc::new(move || {
				called.fetch_add(1, Ordering::SeqCst);
			}) as Arc<dyn Fn() + Send + Sync>
		};
		let mut w = file_watcher(dir.path().to_path_buf(), Vec::new(), cb);
		w.set_interval(Duration::from_millis(100));
		w.start();
		thread::sleep(Duration::from_millis(50));
		std::fs::write(&file, b"changed").unwrap();
		thread::sleep(Duration::from_millis(250));
		w.stop();
		assert!(
			called.load(Ordering::SeqCst) > 0,
			"expected onChange to fire"
		);
	}

	#[test]
	fn no_change_no_fire() {
		let dir = tempfile::tempdir().unwrap();
		std::fs::write(dir.path().join("stable.txt"), b"ok").unwrap();
		snapshot_store::clear();
		let called = Arc::new(AtomicI32::new(0));
		let cb = {
			let called = called.clone();
			Arc::new(move || {
				called.fetch_add(1, Ordering::SeqCst);
			}) as Arc<dyn Fn() + Send + Sync>
		};
		let mut w = file_watcher(dir.path().to_path_buf(), Vec::new(), cb);
		w.set_interval(Duration::from_millis(100));
		w.start();
		thread::sleep(Duration::from_millis(350));
		w.stop();
		assert_eq!(called.load(Ordering::SeqCst), 0);
	}

	#[test]
	fn ignore_directory_pattern() {
		let dir = tempfile::tempdir().unwrap();
		let sub = dir.path().join("ignored");
		std::fs::create_dir(&sub).unwrap();
		let file = sub.join("data.txt");
		std::fs::write(&file, b"init").unwrap();
		snapshot_store::clear();
		let called = Arc::new(AtomicI32::new(0));
		let cb = {
			let called = called.clone();
			Arc::new(move || {
				called.fetch_add(1, Ordering::SeqCst);
			}) as Arc<dyn Fn() + Send + Sync>
		};
		let mut w = file_watcher(dir.path().to_path_buf(), vec!["ignored".into()], cb);
		w.set_interval(Duration::from_millis(100));
		w.start();
		thread::sleep(Duration::from_millis(50));
		std::fs::write(&file, b"changed").unwrap();
		thread::sleep(Duration::from_millis(250));
		w.stop();
		assert_eq!(called.load(Ordering::SeqCst), 0);
	}

	#[test]
	fn ignore_glob_pattern() {
		let dir = tempfile::tempdir().unwrap();
		let file = dir.path().join("app.log");
		std::fs::write(&file, b"init").unwrap();
		snapshot_store::clear();
		let called = Arc::new(AtomicI32::new(0));
		let cb = {
			let called = called.clone();
			Arc::new(move || {
				called.fetch_add(1, Ordering::SeqCst);
			}) as Arc<dyn Fn() + Send + Sync>
		};
		let mut w = file_watcher(dir.path().to_path_buf(), vec!["*.log".into()], cb);
		w.set_interval(Duration::from_millis(100));
		w.start();
		thread::sleep(Duration::from_millis(50));
		std::fs::write(&file, b"changed").unwrap();
		thread::sleep(Duration::from_millis(250));
		w.stop();
		assert_eq!(called.load(Ordering::SeqCst), 0);
	}

	#[test]
	fn double_start_no_leak() {
		let dir = tempfile::tempdir().unwrap();
		let cb = Arc::new(|| {}) as Arc<dyn Fn() + Send + Sync>;
		let mut w = file_watcher(dir.path().to_path_buf(), Vec::new(), cb);
		w.set_interval(Duration::from_millis(100));
		w.start();
		w.start();
		w.stop();
	}

	#[test]
	fn stop_before_start_does_not_panic() {
		let dir = tempfile::tempdir().unwrap();
		let cb = Arc::new(|| {}) as Arc<dyn Fn() + Send + Sync>;
		let w = file_watcher(dir.path().to_path_buf(), Vec::new(), cb);
		w.stop();
	}

	#[test]
	fn match_ignore_path_traversal() {
		assert!(!match_ignore("test", "test", "../secret"));
		assert!(!match_ignore("test", "test", "/etc/passwd"));
	}

	#[test]
	fn match_ignore_exact_and_glob() {
		assert!(match_ignore("node_modules", "node_modules", "node_modules"));
		assert!(match_ignore("app.log", "app.log", "*.log"));
		assert!(!match_ignore("app.txt", "app.txt", "*.log"));
	}

	#[allow(dead_code)]
	fn defer_stop(w: &FileWatcher) {
		let cancel = {
			let g = w.mu.lock().unwrap_or_else(|e| e.into_inner());
			g.cancel.clone()
		};
		if let Some(c) = cancel {
			c.store(true, Ordering::Relaxed);
		}
	}
}
