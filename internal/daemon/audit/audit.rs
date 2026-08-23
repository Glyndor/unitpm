//! JSON-lines audit log for destructive daemon actions.
//!
//! Records every `start`, `stop`, `delete`, `reload`, `restart`, `reset` and
//! `flush` for compliance and post-mortem forensics. The log is **on in system
//! mode** (`/var/log/glyndor/unitpm/audit.log`) and **off in user mode**,
//! where the daemon is already scoped to a single user.
//!
//! Mirrors `internal/daemon/audit/audit.go`. The [`Logger::disabled`]
//! sentinel is a no-op so callers can hand a single `Arc<Logger>` to the
//! handler registry and let it stay inert when audit is intentionally off.

use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, OnceLock};

use crate::jsonx;

/// One JSONL line on disk.
#[derive(Debug, Clone, PartialEq, serde::Serialize, serde::Deserialize)]
pub struct Event {
	#[serde(default, skip_serializing_if = "String::is_empty")]
	pub time: String,
	pub action: String,
	#[serde(default, skip_serializing_if = "String::is_empty")]
	pub uid: String,
	#[serde(default, skip_serializing_if = "String::is_empty")]
	pub gid: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub pid: Option<i32>,
	pub target: String,
	#[serde(default, skip_serializing_if = "String::is_empty")]
	pub name: String,
	#[serde(default, skip_serializing_if = "String::is_empty")]
	pub ns: String,
	pub success: bool,
	#[serde(default, skip_serializing_if = "String::is_empty")]
	pub error: String,
}

impl Event {
	/// Build an event with `time` filled to the current instant.
	#[must_use]
	pub fn now(action: impl Into<String>, target: impl Into<String>) -> Self {
		Self {
			time: now_rfc3339_nano(),
			action: action.into(),
			uid: String::new(),
			gid: String::new(),
			pid: None,
			target: target.into(),
			name: String::new(),
			ns: String::new(),
			success: true,
			error: String::new(),
		}
	}
}

/// JSON-lines audit logger. `log` is a no-op when no writer is set; the
/// sentinel [`Logger::disabled`] returns a shared no-op [`Arc`] so the
/// registry can hold a single reference.
pub struct Logger {
	inner: Mutex<Option<Box<dyn Write + Send>>>,
	path: Option<PathBuf>,
}

static DISABLED: OnceLock<Arc<Logger>> = OnceLock::new();

impl Logger {
	/// Process-wide disabled sentinel. Every `log` is a no-op and `is_enabled`
	/// returns `false`. Intended for user-mode daemons where the audit log is
	/// intentionally off.
	#[must_use]
	pub fn disabled() -> Arc<Self> {
		DISABLED
			.get_or_init(|| {
				Arc::new(Self {
					inner: Mutex::new(None),
					path: None,
				})
			})
			.clone()
	}

	/// Open the audit log at `path`. Creates the parent directory (`0755`)
	/// and the file (`0600`) if missing. Returns a disabled logger on any
	/// filesystem error so the daemon never refuses to start because of audit
	/// setup.
	#[must_use]
	pub fn open(path: impl AsRef<Path>) -> Arc<Self> {
		let path = path.as_ref();
		match Self::open_inner(path) {
			Some((w, p)) => Arc::new(Self {
				inner: Mutex::new(Some(w)),
				path: Some(p),
			}),
			None => Self::disabled(),
		}
	}

	fn open_inner(path: &Path) -> Option<(Box<dyn Write + Send>, PathBuf)> {
		if path.as_os_str().is_empty() {
			return None;
		}
		let parent = path.parent()?;
		std::fs::create_dir_all(parent).ok()?;
		#[cfg(unix)]
		{
			use std::os::unix::fs::PermissionsExt;
			let _ = std::fs::set_permissions(parent, std::fs::Permissions::from_mode(0o755));
		}
		let f = {
			use std::os::unix::fs::OpenOptionsExt;
			std::fs::OpenOptions::new()
				.create(true)
				.append(true)
				.custom_flags(libc_o_nofollow())
				.open(path)
				.ok()?
		};
		#[cfg(unix)]
		{
			use std::os::unix::fs::PermissionsExt;
			let _ = std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600));
		}
		Some((Box::new(f), path.to_path_buf()))
	}

	/// Write one event. Best-effort: I/O errors are swallowed so a full disk
	/// cannot break the IPC path.
	pub fn log(&self, mut event: Event) {
		if event.time.is_empty() {
			event.time = now_rfc3339_nano();
		}
		let mut guard = self.inner.lock().unwrap_or_else(|e| e.into_inner());
		let Some(writer) = guard.as_mut() else {
			return;
		};
		let Ok(mut bytes) = jsonx::marshal(&event) else {
			return;
		};
		bytes.push(b'\n');
		let _ = writer.write_all(&bytes);
		let _ = writer.flush();
	}

	/// Drop the underlying writer. Best-effort: errors are swallowed.
	pub fn close(&self) {
		let mut guard = self.inner.lock().unwrap_or_else(|e| e.into_inner());
		if let Some(mut w) = guard.take() {
			let _ = w.flush();
		}
	}

	/// True when this logger will actually write to disk.
	#[must_use]
	pub fn is_enabled(&self) -> bool {
		self.inner
			.lock()
			.unwrap_or_else(|e| e.into_inner())
			.is_some()
	}

	/// Filesystem path the logger writes to. `None` for a disabled logger.
	#[must_use]
	pub fn path(&self) -> Option<&Path> {
		self.path.as_deref()
	}
}

/// `O_NOFOLLOW` constant for `OpenOptionsExt::custom_flags`. Mirrors the Go
/// `syscall.O_NOFOLLOW` the original audit log uses to refuse symlinked log
/// files. Stable across supported targets (Linux glibc / musl, macOS).
#[cfg(unix)]
fn libc_o_nofollow() -> i32 {
	0x20000
}

#[cfg(not(unix))]
fn libc_o_nofollow() -> i32 {
	0
}

fn now_rfc3339_nano() -> String {
	use std::time::{SystemTime, UNIX_EPOCH};

	let now = SystemTime::now()
		.duration_since(UNIX_EPOCH)
		.unwrap_or_default();
	let secs = now.as_secs() as i64;
	let nanos = now.subsec_nanos();

	let (year, month, day, hour, minute, second) = epoch_to_civil(secs);
	let nanos9 = format!("{nanos:09}");
	format!("{year:04}-{month:02}-{day:02}T{hour:02}:{minute:02}:{second:02}.{nanos9}Z")
}

/// Civil-from-days — RFC3339 wants UTC seconds, so we split the epoch into
/// calendar fields and skip the timezone arithmetic entirely. Howard Hinnant's
/// `days_from_civil` algorithm, inlined for `std::time::SystemTime` callers.
fn epoch_to_civil(secs: i64) -> (i32, u32, u32, u32, u32, u32) {
	let days = secs.div_euclid(86_400);
	let secs_of_day = secs.rem_euclid(86_400) as u32;
	let hour = secs_of_day / 3600;
	let minute = (secs_of_day / 60) % 60;
	let second = secs_of_day % 60;

	let z = days + 719_468;
	let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
	let doe = (z - era * 146_097) as u64;
	let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146_096) / 365;
	let y = yoe as i64 + era * 400;
	let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	let mp = (5 * doy + 2) / 153;
	let d = doy - (153 * mp + 2) / 5 + 1;
	let m = if mp < 10 { mp + 3 } else { mp - 9 };
	let y = if m <= 2 { y + 1 } else { y };

	(y as i32, m as u32, d as u32, hour, minute, second)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn disabled_log_is_noop() {
		Logger::disabled().log(Event::now("x", "y"));
	}

	#[test]
	fn disabled_is_not_enabled() {
		assert!(!Logger::disabled().is_enabled());
	}

	#[test]
	fn close_on_disabled_is_noop() {
		Logger::disabled().close();
	}

	#[test]
	fn open_writes_events() {
		let dir = tempfile::tempdir().expect("tempdir");
		let path = dir.path().join("audit.log");
		let log = Logger::open(&path);

		log.log(Event {
			action: "start".into(),
			uid: "1000".into(),
			pid: Some(1234),
			target: "abc".into(),
			name: "api".into(),
			ns: "default".into(),
			success: true,
			..Event::now("", "")
		});
		log.log(Event {
			action: "delete".into(),
			uid: "1000".into(),
			pid: Some(1234),
			target: "xyz".into(),
			success: false,
			error: "not found".into(),
			..Event::now("", "")
		});
		log.close();

		let data = std::fs::read(&path).expect("read audit log");
		let text = std::str::from_utf8(&data).expect("utf8");
		let lines: Vec<&str> = text.lines().collect();
		assert_eq!(lines.len(), 2, "expected 2 lines, got {text}");

		let first: Event = jsonx::unmarshal(lines[0].as_bytes()).expect("parse line 1");
		assert_eq!(first.action, "start");
		assert!(first.success);
		assert!(!first.time.is_empty());

		let second: Event = jsonx::unmarshal(lines[1].as_bytes()).expect("parse line 2");
		assert_eq!(second.action, "delete");
		assert!(!second.success);
		assert_eq!(second.error, "not found");
	}

	#[test]
	fn open_bad_path_returns_disabled() {
		let log = Logger::open("/proc/nonwritable/audit.log");
		assert!(!log.is_enabled());
	}

	#[test]
	fn open_empty_path_returns_disabled() {
		let log = Logger::open("");
		assert!(!log.is_enabled());
	}

	#[test]
	fn open_creates_file_with_0600() {
		#[cfg(unix)]
		{
			use std::os::unix::fs::PermissionsExt;

			let dir = tempfile::tempdir().expect("tempdir");
			let path = dir.path().join("audit.log");
			let log = Logger::open(&path);
			log.log(Event {
				action: "start".into(),
				success: true,
				..Event::now("", "")
			});
			log.close();

			let meta = std::fs::metadata(&path).expect("metadata");
			assert_eq!(
				meta.permissions().mode() & 0o777,
				0o600,
				"expected 0600 perms"
			);
		}
	}
}
