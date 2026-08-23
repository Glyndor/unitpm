//! Timestamped log writer and lifecycle banner emitter.
//!
//! Mirrors `internal/daemon/manager/logwriter.go`. The writer buffers
//! partial lines and emits them as soon as a newline is seen (or after a
//! 1 MiB flush, whichever comes first), with a `YYYY-MM-DD HH:MM:SS `
//! prefix. The banner is the same 3-line marker (`===` / middle / `===`)
//! that ops staff grep for in incident response.
//!
//! Rotation state is plumbed through
//! [`timestamp_writer::TimestampWriter::new_rotating`] so the daemon-wide
//! ticker (`Manager::rotate_loop`) can drive rotation on a writer that
//! owns a path. The writer itself never triggers rotation inline — that
//! optimisation was dropped during the port because `&mut self` writes
//! cannot overlap with `&self` ticker reads without interior mutability
//! or an extra mutex, and the ticker alone is correct.

use std::io;

/// Fixed column width of the lifecycle banner block.
pub const BANNER_WIDTH: usize = 80;

/// Write a 3-line lifecycle marker (`===` / middle / `===`) to `w`. The
/// middle line carries `event` on the left and the current timestamp on
/// the right, padded with `=` to `BANNER_WIDTH`.
pub fn write_banner<W: io::Write>(w: &mut W, event: &str, detail: &str) {
	let ts = timestamp();
	let sep = "=".repeat(BANNER_WIDTH);

	let mut left = String::from("==  ");
	left.push_str(event);
	if !detail.is_empty() {
		left.push_str("  ");
		left.push_str(detail);
	}
	left.push_str("  ");
	let right = format!("  {ts}  ==");

	let fill_n = BANNER_WIDTH.saturating_sub(left.len() + right.len());
	let fill_n = fill_n.max(4);
	let mid = format!("{left}{}{right}", "=".repeat(fill_n));

	let _ = w.write_all(sep.as_bytes());
	let _ = w.write_all(b"\n");
	let _ = w.write_all(mid.as_bytes());
	let _ = w.write_all(b"\n");
	let _ = w.write_all(sep.as_bytes());
	let _ = w.write_all(b"\n");
}

fn timestamp() -> String {
	format_now()
}

pub(crate) fn format_now() -> String {
	use std::time::{SystemTime, UNIX_EPOCH};
	let now = SystemTime::now()
		.duration_since(UNIX_EPOCH)
		.map(|d| d.as_secs())
		.unwrap_or(0);
	let secs_in_day = 86_400u64;
	let days = (now / secs_in_day) as i64;
	let secs_today = now % secs_in_day;
	let hh = secs_today / 3600;
	let mm = (secs_today / 60) % 60;
	let ss = secs_today % 60;

	// Days since 1970-01-01 → Y-M-D using the algorithm from Howard Hinnant.
	let (y, m, d) = days_to_ymd(days);
	format!("{y:04}-{m:02}-{d:02} {hh:02}:{mm:02}:{ss:02}")
}

fn days_to_ymd(days_since_epoch: i64) -> (i64, u32, u32) {
	let z = days_since_epoch + 719_468;
	let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
	let doe = (z - era * 146_097) as u64;
	let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
	let y = yoe as i64 + era * 400;
	let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	let mp = (5 * doy + 2) / 153;
	let d = (doy - (153 * mp + 2) / 5 + 1) as u32;
	let m = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
	let y = if m <= 2 { y + 1 } else { y };
	(y, m, d)
}

pub mod timestamp_writer {
	//! Per-write log helper.

	use std::fs::File;
	use std::io::{self, Write};
	use std::sync::Mutex;

	use crate::daemon::manager::rotate::{rotate_now_cfg, RotateConfig};

	/// 1 MiB; partial-line buffer cap. A single unwritten line longer than
	/// this triggers a flush-with-newline so a runaway child can't OOM the
	/// daemon.
	const MAX_LOG_BUF: usize = 1 << 20;

	/// Timestamp-prefixed line writer. The writer holds a reference to the
	/// underlying sink; rotation is opted into by passing a `path` to the
	/// rotating constructor.
	pub struct TimestampWriter {
		inner: Box<dyn Write + Send + Sync>,
		buf: Vec<u8>,
		/// Serialises `Write` against [`Self::maybe_rotate`]. The two paths
		/// would otherwise race on the `buf` partial-line state — `Write`
		/// owns it during a call, but `maybe_rotate` only touches the file
		/// on disk, so the lock can be a `try_lock` from inside `Write` and
		/// from `maybe_rotate`. `&mut self` writes also take it briefly.
		mu: Mutex<()>,
		/// Rotation state. `path == ""` disables in-writer rotation
		/// entirely (used by tests that wrap a buffer).
		path: String,
		/// Cached env-driven rotation config; resolved once at construction
		/// so writes do not re-parse env vars on every byte.
		rotate_cfg: RotateConfig,
		/// Unix-nanos of the last successful rotation. Zero means "not
		/// anchored yet" — used to seed the age trigger.
		last_rotate_nanos: std::sync::atomic::AtomicI64,
	}

	impl TimestampWriter {
		/// Wrap a sink that is *not* backed by a file. Rotation is disabled.
		pub fn new(inner: Box<dyn Write + Send + Sync>) -> Self {
			Self {
				inner,
				buf: Vec::new(),
				mu: Mutex::new(()),
				path: String::new(),
				rotate_cfg: RotateConfig::default(),
				last_rotate_nanos: std::sync::atomic::AtomicI64::new(0),
			}
		}

		/// Wrap a sink that is backed by a file at `path`. The daemon-wide
		/// ticker ([`crate::Manager`]) can call [`Self::maybe_rotate`] on
		/// this writer to rotate the file.
		pub fn new_rotating(
			inner: Box<dyn Write + Send + Sync>,
			path: String,
			cfg: RotateConfig,
		) -> Self {
			let s = Self {
				inner,
				buf: Vec::new(),
				mu: Mutex::new(()),
				path,
				rotate_cfg: cfg,
				last_rotate_nanos: std::sync::atomic::AtomicI64::new(0),
			};
			s.last_rotate_nanos
				.store(now_unix_nanos(), std::sync::atomic::Ordering::Relaxed);
			s
		}

		/// Underlying path, or empty when rotation is disabled.
		#[must_use]
		pub fn path(&self) -> &str {
			&self.path
		}

		/// Run a rotation check. Returns `true` when a rotation happened
		/// (useful in tests; production callers ignore the return).
		pub fn maybe_rotate(&self) -> bool {
			if self.path.is_empty() {
				return false;
			}
			let Ok(_g) = self.mu.try_lock() else {
				return false;
			};
			let anchor = nanos_to_instant(
				self.last_rotate_nanos
					.load(std::sync::atomic::Ordering::Relaxed),
			);
			if rotate_now_cfg(&self.path, &self.rotate_cfg, anchor) {
				self.last_rotate_nanos
					.store(now_unix_nanos(), std::sync::atomic::Ordering::Relaxed);
				true
			} else {
				false
			}
		}

		/// Number of bytes currently held in the partial-line buffer.
		#[must_use]
		pub fn buffered(&self) -> usize {
			self.buf.len()
		}

		/// Re-anchor the age trigger to "now". Called by the daemon
		/// manager after `setupLogs` opens the writer so the age trigger
		/// only fires after `max_age` has elapsed since the writer opened,
		/// not since some prior rotation.
		pub fn reset_age_anchor(&self) {
			self.last_rotate_nanos
				.store(now_unix_nanos(), std::sync::atomic::Ordering::Relaxed);
		}
	}

	impl Write for TimestampWriter {
		fn write(&mut self, p: &[u8]) -> io::Result<usize> {
			// No lock here: `&mut self` provides exclusive access. The
			// ticker calls `maybe_rotate(&self)` and is excluded from
			// overlapping with a write because the writer holds `&mut self`
			// for the duration of the call.
			self.buf.extend_from_slice(p);
			let ts = ts_prefix();
			let mut out: Vec<u8> = Vec::new();
			loop {
				let Some(idx) = self.buf.iter().position(|&b| b == b'\n') else {
					if self.buf.len() > MAX_LOG_BUF {
						out.extend_from_slice(ts.as_bytes());
						out.extend_from_slice(&self.buf);
						out.push(b'\n');
						self.buf.clear();
					}
					break;
				};
				out.extend_from_slice(ts.as_bytes());
				out.extend_from_slice(&self.buf[..=idx]);
				self.buf.drain(..=idx);
			}
			if !out.is_empty() {
				self.inner.write_all(&out)?;
			}
			Ok(p.len())
		}

		fn flush(&mut self) -> io::Result<()> {
			self.inner.flush()
		}
	}

	fn now_unix_nanos() -> i64 {
		use std::time::{SystemTime, UNIX_EPOCH};
		let d = SystemTime::now()
			.duration_since(UNIX_EPOCH)
			.unwrap_or_default();
		d.as_secs() as i64 * 1_000_000_000 + d.subsec_nanos() as i64
	}

	/// Translate a unix-nanos timestamp back into an `Instant`. The
	/// `Instant` epoch is monotonic; we reconstruct "now - elapsed" where
	/// `elapsed` is `now.duration_since(UNIX_EPOCH)` minus the recorded
	/// `last_rotate_nanos`.
	fn nanos_to_instant(nanos: i64) -> std::time::Instant {
		use std::time::{Instant, SystemTime, UNIX_EPOCH};
		if nanos == 0 {
			return Instant::now();
		}
		let now_sys = SystemTime::now()
			.duration_since(UNIX_EPOCH)
			.unwrap_or_default();
		let now_nanos = now_sys.as_secs() as i64 * 1_000_000_000 + now_sys.subsec_nanos() as i64;
		let delta_nanos = (now_nanos - nanos).max(0) as u64;
		let now = Instant::now();
		now.checked_sub(std::time::Duration::from_nanos(delta_nanos))
			.unwrap_or(now)
	}

	fn ts_prefix() -> String {
		super::format_now() + " "
	}

	/// Convenience: turn a borrowed `File` into a [`TimestampWriter`]
	/// backed by the file's path with the supplied rotate config.
	pub fn wrap_file(file: File, path: String, cfg: RotateConfig) -> TimestampWriter {
		TimestampWriter::new_rotating(Box::new(file), path, cfg)
	}

	/// Same as [`wrap_file`] but with no rotation config (used in tests).
	pub fn wrap_buffer(buf: std::sync::Arc<std::sync::Mutex<Vec<u8>>>) -> TimestampWriter {
		TimestampWriter::new(Box::new(BufferSink(buf)))
	}

	struct BufferSink(std::sync::Arc<std::sync::Mutex<Vec<u8>>>);

	impl Write for BufferSink {
		fn write(&mut self, p: &[u8]) -> io::Result<usize> {
			self.0.lock().unwrap().extend_from_slice(p);
			Ok(p.len())
		}
		fn flush(&mut self) -> io::Result<()> {
			Ok(())
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::io::Write;

	#[test]
	fn banner_three_lines() {
		let mut buf = Vec::new();
		write_banner(&mut buf, "STARTED", "");
		let s = std::str::from_utf8(&buf).unwrap();
		let lines: Vec<&str> = s.split('\n').filter(|l| !l.is_empty()).collect();
		assert_eq!(lines.len(), 3, "expected 3 banner lines, got {lines:?}");
	}

	#[test]
	fn banner_middle_has_event_and_timestamp() {
		let mut buf = Vec::new();
		write_banner(&mut buf, "STARTED", "");
		let s = std::str::from_utf8(&buf).unwrap();
		let lines: Vec<&str> = s.split('\n').collect();
		assert!(lines[1].contains("STARTED"));
		assert!(lines[1].ends_with("=="));
	}

	#[test]
	fn banner_middle_width_matches_constant() {
		let mut buf = Vec::new();
		write_banner(&mut buf, "AUTO-RESTART", "attempt=3 delay=4s");
		let s = std::str::from_utf8(&buf).unwrap();
		let lines: Vec<&str> = s.split('\n').collect();
		assert!(lines[1].len() >= BANNER_WIDTH);
	}

	#[test]
	fn banner_with_detail_includes_detail() {
		let mut buf = Vec::new();
		write_banner(&mut buf, "AUTO-RESTART", "attempt=3 delay=4s");
		let s = std::str::from_utf8(&buf).unwrap();
		assert!(s.contains("AUTO-RESTART"));
		assert!(s.contains("attempt=3 delay=4s"));
	}

	#[test]
	fn timestamp_writer_single_line() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink.clone());
		w.write_all(b"hello world\n").unwrap();
		let data = String::from_utf8(sink.lock().unwrap().clone()).unwrap();
		assert!(data.ends_with(" hello world\n"));
		assert_eq!(data.len(), 20 + b"hello world\n".len());
	}

	#[test]
	fn timestamp_writer_multiple_lines() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink.clone());
		w.write_all(b"line1\nline2\nline3\n").unwrap();
		let data = String::from_utf8(sink.lock().unwrap().clone()).unwrap();
		let lines: Vec<&str> = data.split('\n').filter(|l| !l.is_empty()).collect();
		assert_eq!(lines.len(), 3);
	}

	#[test]
	fn timestamp_writer_partial_lines_buffer() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink.clone());
		w.write_all(b"hel").unwrap();
		assert!(sink.lock().unwrap().is_empty());
		w.write_all(b"lo\n").unwrap();
		let data = String::from_utf8(sink.lock().unwrap().clone()).unwrap();
		assert!(data.ends_with(" hello\n"));
	}

	#[test]
	fn timestamp_writer_batch_single_write() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink.clone());
		w.write_all(b"a\nb\n").unwrap();
		let data = String::from_utf8(sink.lock().unwrap().clone()).unwrap();
		let lines: Vec<&str> = data.split('\n').filter(|l| !l.is_empty()).collect();
		assert_eq!(lines.len(), 2);
	}

	#[test]
	fn timestamp_writer_large_buffer_flushes() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink.clone());
		let big = "x".repeat((1 << 20) + 1);
		w.write_all(big.as_bytes()).unwrap();
		let data = String::from_utf8(sink.lock().unwrap().clone()).unwrap();
		assert!(!data.is_empty());
		assert!(data.ends_with('\n'));
	}

	#[test]
	fn timestamp_writer_empty_write_noop() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink);
		let n = w.write(&[]).unwrap();
		assert_eq!(n, 0);
	}

	#[test]
	fn timestamp_writer_rotating_truncates_on_threshold() {
		use std::fs::OpenOptions;
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		std::fs::write(&path, "x".repeat(500)).unwrap();
		let f = OpenOptions::new().append(true).open(&path).unwrap();
		let cfg = crate::daemon::manager::rotate::RotateConfig {
			max_bytes: 100,
			keep: 3,
			delay_compress: true,
			notif_empty: true,
			..Default::default()
		};
		let w = timestamp_writer::wrap_file(f, path.to_str().unwrap().to_string(), cfg);
		w.maybe_rotate();
		assert!(dir.path().join("stdout.log.1").exists());
		assert!(!dir.path().join("stdout.log.1.gz").exists());
	}

	#[test]
	fn timestamp_writer_no_rotate_below_threshold() {
		use std::fs::OpenOptions;
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		std::fs::write(&path, b"small").unwrap();
		let f = OpenOptions::new().append(true).open(&path).unwrap();
		let cfg = crate::daemon::manager::rotate::RotateConfig {
			max_bytes: 1_000_000,
			keep: 3,
			delay_compress: true,
			notif_empty: true,
			..Default::default()
		};
		let w = timestamp_writer::wrap_file(f, path.to_str().unwrap().to_string(), cfg);
		w.maybe_rotate();
		assert!(!dir.path().join("stdout.log.1.gz").exists());
	}

	#[test]
	fn timestamp_writer_disabled_with_empty_path() {
		let sink: std::sync::Arc<std::sync::Mutex<Vec<u8>>> = Default::default();
		let mut w = timestamp_writer::wrap_buffer(sink.clone());
		w.maybe_rotate();
		w.write_all(b"hello\n").unwrap();
		let data = String::from_utf8(sink.lock().unwrap().clone()).unwrap();
		assert!(data.ends_with(" hello\n"));
	}
}
