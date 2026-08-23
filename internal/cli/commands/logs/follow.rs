//! Follow-mode tail: tail every source, push new entries into a heap, and
//! flush entries whose timestamp is older than `now - flushDelay` so
//! out-of-order arrivals get re-sorted by write-time.
//!
//! This module owns three concerns:
//!
//!   - the per-source follower ([`tail_follow`]) that turns a tailing
//!     reader into a stream of [`FollowMessage`]s
//!   - the merger ([`follow_merge`]) that maintains the small-window heap
//!     and emits flushed entries
//!   - the file-appears-later waiter ([`wait_open`])

use std::cmp::Ordering;
use std::collections::BinaryHeap;
use std::fs::{File, OpenOptions};
use std::io::{BufRead, BufReader, Seek, SeekFrom, Write as _};
use std::sync::mpsc::{channel, Sender};
use std::thread;
use std::time::Duration;

use super::entry::{self, Entry};
use super::merge::{format_entry, Filter, StreamSource};

/// How long entries are buffered in the heap before they qualify for
/// emission. Tuned in the Go reference; doubled here for tests that
/// inject events manually.
pub const FLUSH_DELAY: Duration = Duration::from_millis(200);

/// Sleep function used by follow loops. Injectable so tests can
/// substitute a no-op sleeper — production uses [`std::thread::sleep`].
pub type Sleeper = fn(Duration);

/// One peer's message: a new entry, or an error to surface.
pub enum FollowMessage {
	Entry(Entry),
	Err(String),
}

/// Min-heap on `(ts_unix, seq)`.
pub struct EntryHeap {
	inner: BinaryHeap<HeapEntry>,
}

struct HeapEntry {
	ts_unix: i64,
	seq: u64,
	entry: Entry,
}

impl PartialEq for HeapEntry {
	fn eq(&self, other: &Self) -> bool {
		self.ts_unix == other.ts_unix && self.seq == other.seq
	}
}
impl Eq for HeapEntry {}
impl Ord for HeapEntry {
	fn cmp(&self, other: &Self) -> Ordering {
		// BinaryHeap is a max-heap, so invert the order to get a min-heap.
		other
			.ts_unix
			.cmp(&self.ts_unix)
			.then_with(|| other.seq.cmp(&self.seq))
	}
}
impl PartialOrd for HeapEntry {
	fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
		Some(self.cmp(other))
	}
}

impl EntryHeap {
	#[must_use]
	pub fn new() -> Self {
		Self {
			inner: BinaryHeap::new(),
		}
	}
	#[must_use]
	pub fn len(&self) -> usize {
		self.inner.len()
	}
	#[must_use]
	pub fn is_empty(&self) -> bool {
		self.inner.is_empty()
	}
	pub fn push(&mut self, entry: Entry) {
		let ts_unix = entry.ts_unix.unwrap_or(0);
		self.inner.push(HeapEntry {
			ts_unix,
			seq: entry.seq,
			entry,
		});
	}
	pub fn pop(&mut self) -> Option<Entry> {
		self.inner.pop().map(|h| h.entry)
	}
	pub fn peek(&self) -> Option<&Entry> {
		self.inner.peek().map(|h| &h.entry)
	}
}

impl Default for EntryHeap {
	fn default() -> Self {
		Self::new()
	}
}

/// Block until `path` exists or `sleeper` allows shutdown.
pub fn wait_open(path: &str, sleeper: Sleeper) -> Result<File, std::io::Error> {
	loop {
		match OpenOptions::new().read(true).open(path) {
			Ok(f) => return Ok(f),
			Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
				sleeper(Duration::from_millis(500));
			}
			Err(e) => return Err(e),
		}
	}
}

/// Tail `path` and push each new entry to `tx`. The first call reads
/// from end-of-file; subsequent writes are picked up after `sleeper` polls.
pub fn tail_follow(
	source: StreamSource,
	tx: Sender<FollowMessage>,
	sleeper: Sleeper,
) -> Result<(), std::io::Error> {
	let mut f = match wait_open(&source.path, sleeper) {
		Ok(f) => f,
		Err(e) => {
			let _ = tx.send(FollowMessage::Err(format!("{} open: {}", source.label, e)));
			return Err(e);
		}
	};
	f.seek(SeekFrom::End(0))?;
	let mut br = BufReader::new(f);
	let mut seq = source.seq_base;
	let mut pending: Option<Entry> = None;
	let mut banner_buf: Vec<String> = Vec::new();

	loop {
		let mut line = String::new();
		let n = match br.read_line(&mut line) {
			Ok(n) => n,
			Err(e) => {
				let _ = tx.send(FollowMessage::Err(format!("{} read: {}", source.label, e)));
				return Err(e);
			}
		};
		if n == 0 {
			if let Some(e) = pending.take() {
				let _ = tx.send(FollowMessage::Entry(e));
			}
			sleeper(Duration::from_millis(150));
			continue;
		}
		if line.ends_with('\n') {
			line.pop();
		}
		if line.ends_with('\r') {
			line.pop();
		}
		match banner_buf.len() {
			1 => {
				if entry::parse_banner_middle(&line).is_some() {
					banner_buf.push(line.clone());
				} else {
					flush_banner_as_continuation(&mut banner_buf, &mut pending);
					if let Some(e) = handle_line(
						&line,
						&source.label,
						&mut pending,
						&mut banner_buf,
						&mut seq,
					) {
						let _ = tx.send(FollowMessage::Entry(e));
					}
				}
			}
			2 => {
				if entry::is_banner_rule(&line) {
					banner_buf.push(line.clone());
					if let Some(mid) = entry::parse_banner_middle(&banner_buf[1]) {
						if let Some(e) = pending.take() {
							let _ = tx.send(FollowMessage::Entry(e));
						}
						let body = banner_buf.join("\n");
						let _ = tx.send(FollowMessage::Entry(Entry {
							ts_unix: Some(mid),
							label: source.label.clone(),
							body,
							has_ts: true,
							seq,
						}));
						seq += 1;
					}
					banner_buf.clear();
				} else {
					flush_banner_as_continuation(&mut banner_buf, &mut pending);
					if let Some(e) = handle_line(
						&line,
						&source.label,
						&mut pending,
						&mut banner_buf,
						&mut seq,
					) {
						let _ = tx.send(FollowMessage::Entry(e));
					}
				}
			}
			_ => {
				if let Some(e) = handle_line(
					&line,
					&source.label,
					&mut pending,
					&mut banner_buf,
					&mut seq,
				) {
					let _ = tx.send(FollowMessage::Entry(e));
				}
			}
		}
	}
}

fn flush_banner_as_continuation(banner_buf: &mut Vec<String>, pending: &mut Option<Entry>) {
	if banner_buf.is_empty() {
		return;
	}
	if let Some(p) = pending.as_mut() {
		let joined = banner_buf.join("\n");
		p.body.push('\n');
		p.body.push_str(&joined);
	}
	banner_buf.clear();
}

fn handle_line(
	line: &str,
	label: &str,
	pending: &mut Option<Entry>,
	banner_buf: &mut Vec<String>,
	seq: &mut u64,
) -> Option<Entry> {
	if entry::is_banner_rule(line) {
		banner_buf.push(line.to_string());
		return None;
	}
	if let Some(ts) = parse_ts(line) {
		let displaced = pending.take();
		let body = if line.len() > entry::TS_LEN {
			line[entry::TS_LEN..].trim_start().to_string()
		} else {
			String::new()
		};
		*pending = Some(Entry {
			ts_unix: Some(ts),
			label: label.to_string(),
			body,
			has_ts: true,
			seq: *seq,
		});
		*seq += 1;
		return displaced;
	}
	if let Some(p) = pending.as_mut() {
		p.body.push('\n');
		p.body.push_str(line);
	}
	None
}

fn parse_ts(line: &str) -> Option<i64> {
	let (ts, _body, ok) = entry::parse_line(line);
	if ok {
		ts
	} else {
		None
	}
}

/// Open every source, then drive the flush loop until `rx` is closed
/// (all followers have exited). Writes qualifying entries to `w`.
pub fn follow_merge<W: std::io::Write>(
	w: &mut W,
	fs: &Filter,
	sources: Vec<StreamSource>,
	sleeper: Sleeper,
) -> Result<(), std::io::Error> {
	let (tx, rx) = channel::<FollowMessage>();
	let mut handles = Vec::with_capacity(sources.len());
	for s in sources {
		let tx = tx.clone();
		let sleeper_fn = sleeper;
		handles.push(thread::spawn(move || {
			let _ = tail_follow(s, tx, sleeper_fn);
		}));
	}
	drop(tx);

	let mut heap = EntryHeap::new();
	loop {
		match rx.recv_timeout(FLUSH_DELAY / 2) {
			Ok(FollowMessage::Entry(e)) => heap.push(e),
			Ok(FollowMessage::Err(msg)) => {
				let stderr = std::io::stderr();
				let mut err = stderr.lock();
				let _ = writeln!(err, "\x1b[31mfollow error: {msg}\x1b[0m");
			}
			Err(std::sync::mpsc::RecvTimeoutError::Timeout) => {
				// fall through to flush pass
			}
			Err(std::sync::mpsc::RecvTimeoutError::Disconnected) => break,
		}
		let cutoff = now_unix_minus(FLUSH_DELAY);
		while let Some(top) = heap.peek() {
			let top_ts = top.ts_unix.unwrap_or(0);
			if top_ts > cutoff {
				break;
			}
			let entry = heap.pop().unwrap();
			if fs.keep(&entry) {
				writeln!(w, "{}", format_entry(&entry))?;
			}
		}
	}
	for h in handles {
		let _ = h.join();
	}
	Ok(())
}

fn now_unix_minus(d: Duration) -> i64 {
	entry::now_unix() - d.as_secs() as i64
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::commands::logs::entry::Entry as E;

	fn entry(ts: i64, body: &str, seq: u64) -> E {
		E {
			ts_unix: Some(ts),
			label: "STDOUT".into(),
			body: body.into(),
			seq,
			has_ts: true,
		}
	}

	#[test]
	fn entry_heap_orders_by_ts() {
		let mut h = EntryHeap::new();
		h.push(entry(300, "c", 2));
		h.push(entry(100, "a", 0));
		h.push(entry(200, "b", 1));
		let mut bodies = Vec::new();
		while let Some(e) = h.pop() {
			bodies.push(e.body);
		}
		assert_eq!(bodies, vec!["a", "b", "c"]);
	}

	#[test]
	fn entry_heap_tie_break_by_seq() {
		let mut h = EntryHeap::new();
		h.push(entry(100, "second", 5));
		h.push(entry(100, "first", 3));
		assert_eq!(h.pop().unwrap().body, "first");
		assert_eq!(h.pop().unwrap().body, "second");
	}

	#[test]
	fn wait_open_succeeds_when_file_appears() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("delayed.log");
		let path_str = path.to_string_lossy().into_owned();
		let handle = std::thread::spawn({
			let path_str = path_str.clone();
			move || {
				std::thread::sleep(Duration::from_millis(20));
				std::fs::write(&path_str, b"hello\n").unwrap();
			}
		});
		let f = wait_open(&path_str, |_| {}).unwrap();
		drop(f);
		handle.join().unwrap();
	}

	#[test]
	fn wait_open_errors_on_other_io() {
		// The Go reference uses a directory path; on Linux read-only
		// opens on directories succeed, so it cannot distinguish the
		// branches. Pass a path with an embedded NUL byte — every
		// syscall layer rejects this with `InvalidInput`, the path
		// cannot exist in any form, so the loop must return the error
		// immediately without polling.
		let err = wait_open("\0bad", |_| {}).unwrap_err();
		assert_eq!(err.kind(), std::io::ErrorKind::InvalidInput);
	}

	#[test]
	fn flush_loop_emits_in_chronological_order() {
		// Drive the heap-flush path directly without involving real
		// followers — we want to assert ordering and the flush cutoff
		// behaviour, not the file polling.
		let entries = vec![entry(100, "a", 0), entry(200, "b", 1), entry(300, "c", 2)];
		let mut heap = EntryHeap::new();
		for e in entries {
			heap.push(e);
		}
		// Cutoff newer than all entries → all flush.
		let cutoff = 1000i64;
		let mut emitted = Vec::new();
		while let Some(top) = heap.peek() {
			let ts = top.ts_unix.unwrap_or(0);
			if ts > cutoff {
				break;
			}
			emitted.push(heap.pop().unwrap().body);
		}
		assert_eq!(emitted, vec!["a", "b", "c"]);
	}

	#[test]
	fn flush_loop_respects_cutoff() {
		let mut heap = EntryHeap::new();
		heap.push(entry(100, "old", 0));
		heap.push(entry(500, "new", 1));
		let cutoff = 200i64;
		let mut emitted = Vec::new();
		while let Some(top) = heap.peek() {
			let ts = top.ts_unix.unwrap_or(0);
			if ts > cutoff {
				break;
			}
			emitted.push(heap.pop().unwrap().body);
		}
		assert_eq!(emitted, vec!["old"]);
		assert_eq!(heap.len(), 1);
	}
}
