//! Log rotation.
//!
//! Mirrors `internal/daemon/manager/rotate.go`. Two schemes are supported:
//!
//! - **immediate**: current log → `.1.gz`, `.1.gz` → `.2.gz`, ... — every
//!   rotation compresses right away.
//! - **delay-compress**: current log → `.1` (plain), `.1` → `.2.gz`, ...
//!   — matches `logrotate`'s `delaycompress`. The most recent rotated copy
//!   stays plain-text so a human can `tail` it without `zcat`.
//!
//! Both end with a `copytruncate` of the live file so the daemon's open fd
//! keeps writing to the same inode and never rotates underneath a writer
//! mid-line.

use std::fs::{self, File};
use std::io::{self, Read, Write};
use std::path::Path;
use std::time::{Duration, Instant};

use flate2::write::GzEncoder;
use flate2::Compression;

use crate::env;

/// 50 MiB — matches the Debian package's `logrotate` config.
pub const DEFAULT_MAX_BYTES: i64 = 50 * 1024 * 1024;
/// 12 — matches the Debian package's `rotate 12`.
pub const DEFAULT_KEEP: i32 = 12;
/// 7 days — matches `logrotate`'s `weekly`.
pub const DEFAULT_MAX_AGE: Duration = Duration::from_secs(7 * 24 * 3600);

/// Knobs for one rotation call. All fields read from env at construction.
#[derive(Debug, Clone)]
pub struct RotateConfig {
	/// Maximum live-log size before rotation. Zero disables the size trigger.
	pub max_bytes: i64,
	/// Maximum number of `.gz` archives kept. Minimum 1.
	pub keep: i32,
	/// Maximum age since the writer's anchor before rotation. Zero disables
	/// the age trigger.
	pub max_age: Duration,
	/// When true, follow logrotate's `delaycompress` — leave the most
	/// recent rotated copy plain until the next rotation.
	pub delay_compress: bool,
	/// When true, do not rotate an empty log (matches `notifempty`).
	pub notif_empty: bool,
}

impl Default for RotateConfig {
	fn default() -> Self {
		Self {
			max_bytes: DEFAULT_MAX_BYTES,
			keep: DEFAULT_KEEP,
			max_age: DEFAULT_MAX_AGE,
			delay_compress: true,
			notif_empty: true,
		}
	}
}

/// Snapshot the env-driven knobs once. Mirrors the Go `currentRotateConfig`.
pub fn current_rotate_config() -> RotateConfig {
	let hours = env::int(
		"UNITPM_LOG_MAX_AGE_HOURS",
		(DEFAULT_MAX_AGE.as_secs() / 3600) as i64,
	);
	RotateConfig {
		max_bytes: env::int64("UNITPM_LOG_MAX_BYTES", DEFAULT_MAX_BYTES),
		keep: env::int("UNITPM_LOG_KEEP", DEFAULT_KEEP as i64) as i32,
		max_age: Duration::from_secs((hours as u64) * 3600),
		delay_compress: true,
		notif_empty: true,
	}
}

/// Size-only entry point used at Start time when no anchor exists yet.
pub fn rotate_if_large(path: &str) {
	rotate_now_cfg(path, &current_rotate_config(), Instant::now());
}

/// Same as [`rotate_if_large`] but with a pinned config (used by unit tests).
/// Returns whether rotation actually happened.
pub fn rotate_if_large_cfg(path: &str, cfg: &RotateConfig) -> bool {
	rotate_now_cfg(path, cfg, Instant::now())
}

/// Canonical rotation entry point. Evaluates both size and age triggers.
/// `last_rotate_at` anchors the age check — pass `Instant::now()` to disable
/// the age trigger.
pub fn rotate_now_cfg(path: &str, cfg: &RotateConfig, last_rotate_at: Instant) -> bool {
	let meta = match fs::metadata(path) {
		Ok(m) => m,
		Err(_) => return false,
	};
	if cfg.notif_empty && meta.len() == 0 {
		return false;
	}
	let by_size = cfg.max_bytes > 0 && meta.len() >= cfg.max_bytes as u64;
	let by_age = cfg.max_age > Duration::ZERO && last_rotate_at.elapsed() >= cfg.max_age;
	if !by_size && !by_age {
		return false;
	}
	rotate_chain(path, cfg);
	true
}

fn rotate_chain(path: &str, cfg: &RotateConfig) {
	let keep = cfg.keep.max(1);

	// Drop the oldest compressed archive.
	let oldest = format!("{path}.{keep}.gz");
	if let Err(e) = fs::remove_file(&oldest) {
		if e.kind() != io::ErrorKind::NotFound {
			eprintln!("log-rotate: remove {oldest}: {e}");
		}
	}

	// Shift the compressed chain up. Immediate: starts at .1.gz. Delay: at .2.gz.
	let start_idx = if cfg.delay_compress { 2 } else { 1 };
	for i in (start_idx..=keep - 1).rev() {
		let src = format!("{path}.{i}.gz");
		let dst = format!("{path}.{}.gz", i + 1);
		if let Err(e) = fs::rename(&src, &dst) {
			if e.kind() != io::ErrorKind::NotFound {
				eprintln!("log-rotate: rename {src} → {dst}: {e}");
			}
		}
	}

	if cfg.delay_compress {
		let plain1 = format!("{path}.1");
		if Path::new(&plain1).exists() {
			if let Err(e) = compress_file(&plain1, &format!("{path}.2.gz")) {
				eprintln!("log-rotate: compress {plain1}: {e}");
				return;
			}
			if let Err(e) = fs::remove_file(&plain1) {
				if e.kind() != io::ErrorKind::NotFound {
					eprintln!("log-rotate: remove {plain1}: {e}");
				}
			}
		}
		if let Err(e) = copy_file(path, &plain1) {
			eprintln!("log-rotate: copy {path} → {plain1}: {e}");
			return;
		}
	} else if let Err(e) = compress_file(path, &format!("{path}.1.gz")) {
		eprintln!("log-rotate: compress {path}: {e}");
		return;
	}

	if let Err(e) = fs::OpenOptions::new()
		.write(true)
		.open(path)
		.and_then(|f| f.set_len(0))
	{
		eprintln!("log-rotate: truncate {path}: {e}");
	}
}

fn compress_file(src: &str, dst: &str) -> io::Result<()> {
	let mut input = File::open(src)?;
	let mut data = Vec::new();
	input.read_to_end(&mut data)?;
	let out = File::create(dst)?;
	let mut enc = GzEncoder::new(out, Compression::best());
	enc.write_all(&data)?;
	enc.finish()?;
	Ok(())
}

fn copy_file(src: &str, dst: &str) -> io::Result<()> {
	let mut input = File::open(src)?;
	let mut data = Vec::new();
	input.read_to_end(&mut data)?;
	let mut out = File::create(dst)?;
	out.write_all(&data)?;
	Ok(())
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::io::Write;

	fn read_gz(path: &str) -> String {
		let mut f = File::open(path).expect("open");
		let mut dec = flate2::read::GzDecoder::new(&mut f);
		let mut s = String::new();
		std::io::Read::read_to_string(&mut dec, &mut s).expect("read");
		s
	}

	#[test]
	fn below_threshold_does_not_rotate() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		fs::write(&path, b"small").unwrap();
		let cfg = RotateConfig {
			max_bytes: 20,
			keep: 2,
			delay_compress: true,
			notif_empty: true,
			..RotateConfig::default()
		};
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert!(!path.with_extension("log.1").exists());
		assert!(!path.with_extension("log.1.gz").exists());
	}

	#[test]
	fn above_threshold_truncates_and_compresses_immediate() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		fs::write(&path, "x".repeat(30)).unwrap();
		let cfg = RotateConfig {
			max_bytes: 20,
			keep: 2,
			delay_compress: false,
			notif_empty: true,
			..RotateConfig::default()
		};
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		let gz = dir.path().join("stdout.log.1.gz");
		assert!(gz.exists());
		let meta = fs::metadata(&path).unwrap();
		assert_eq!(meta.len(), 0);
		assert_eq!(read_gz(gz.to_str().unwrap()), "x".repeat(30));
	}

	#[test]
	fn delay_compress_first_rotation_keeps_plain() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		fs::write(&path, "a".repeat(30)).unwrap();
		let cfg = RotateConfig {
			max_bytes: 20,
			keep: 12,
			delay_compress: true,
			notif_empty: true,
			..RotateConfig::default()
		};
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		let plain = dir.path().join("stdout.log.1");
		let gz = dir.path().join("stdout.log.1.gz");
		assert!(plain.exists());
		assert!(!gz.exists());
		let data = fs::read(&plain).unwrap();
		assert_eq!(String::from_utf8(data).unwrap(), "a".repeat(30));
	}

	#[test]
	fn delay_compress_chain_grows() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		let cfg = RotateConfig {
			max_bytes: 20,
			keep: 12,
			delay_compress: true,
			notif_empty: true,
			..RotateConfig::default()
		};
		fs::write(&path, "a".repeat(30)).unwrap();
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		fs::write(&path, "b".repeat(30)).unwrap();
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert_eq!(
			fs::read(dir.path().join("stdout.log.1")).unwrap(),
			b"b".repeat(30)
		);
		assert!(!dir.path().join("stdout.log.1.gz").exists());
		assert_eq!(
			read_gz(dir.path().join("stdout.log.2.gz").to_str().unwrap()),
			"a".repeat(30)
		);

		fs::write(&path, "c".repeat(30)).unwrap();
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert_eq!(
			fs::read(dir.path().join("stdout.log.1")).unwrap(),
			b"c".repeat(30)
		);
		assert_eq!(
			read_gz(dir.path().join("stdout.log.2.gz").to_str().unwrap()),
			"b".repeat(30)
		);
		assert_eq!(
			read_gz(dir.path().join("stdout.log.3.gz").to_str().unwrap()),
			"a".repeat(30)
		);
	}

	#[test]
	fn notif_empty_skips_zero_byte_file() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		fs::write(&path, b"").unwrap();
		let cfg = RotateConfig {
			max_bytes: 20,
			keep: 12,
			delay_compress: true,
			notif_empty: true,
			..RotateConfig::default()
		};
		let rotated = rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert!(!rotated);
	}

	#[test]
	fn age_trigger_fires_below_threshold() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		fs::write(&path, b"not big yet").unwrap();
		let cfg = RotateConfig {
			max_bytes: 1 << 30,
			keep: 12,
			max_age: Duration::from_millis(50),
			delay_compress: true,
			notif_empty: true,
			..RotateConfig::default()
		};
		let anchor = Instant::now() - Duration::from_secs(1);
		let rotated = rotate_now_cfg(path.to_str().unwrap(), &cfg, anchor);
		assert!(rotated, "age trigger should fire when anchor is old");
		assert!(dir.path().join("stdout.log.1").exists());
	}

	#[test]
	fn age_trigger_holds_back_when_anchor_recent() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		fs::write(&path, b"small").unwrap();
		let cfg = RotateConfig {
			max_bytes: 1 << 30,
			keep: 12,
			max_age: Duration::from_secs(3600),
			delay_compress: true,
			notif_empty: true,
			..RotateConfig::default()
		};
		let rotated = rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert!(!rotated);
	}

	#[test]
	fn second_rotation_shifts_immediate_chain() {
		let dir = tempfile::tempdir().unwrap();
		let path = dir.path().join("stdout.log");
		let cfg = RotateConfig {
			max_bytes: 20,
			keep: 2,
			delay_compress: false,
			notif_empty: true,
			..RotateConfig::default()
		};
		fs::write(&path, "x".repeat(30)).unwrap();
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert_eq!(
			read_gz(dir.path().join("stdout.log.1.gz").to_str().unwrap()),
			"x".repeat(30)
		);

		fs::write(&path, "y".repeat(30)).unwrap();
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		assert_eq!(
			read_gz(dir.path().join("stdout.log.2.gz").to_str().unwrap()),
			"x".repeat(30)
		);
		assert_eq!(
			read_gz(dir.path().join("stdout.log.1.gz").to_str().unwrap()),
			"y".repeat(30)
		);

		fs::write(&path, "z".repeat(30)).unwrap();
		rotate_now_cfg(path.to_str().unwrap(), &cfg, Instant::now());
		// keep=2 → .3.gz must NOT exist (oldest evicted).
		assert!(!dir.path().join("stdout.log.3.gz").exists());
	}
}
