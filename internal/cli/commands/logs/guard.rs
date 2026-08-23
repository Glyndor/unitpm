//! Size guard rails for "read whole file" paths (--all, very large -n).
//!
//! Bounded tail with seek-from-end is unaffected — it never scans more
//! than `n*200` bytes per source, so the user is already protected from
//! the worst case. The 10 MiB / 100 MiB thresholds only matter for
//! `streamMerge`, which reads the file end-to-end.

use std::io::{self, BufRead};

use crate::cli::commands::logs::merge::StreamSource;

/// 10 MiB — emit a warning; prompt when interactive.
pub const WARN_SIZE: i64 = 10 * 1024 * 1024;
/// 100 MiB — refuse to proceed unless `--yes` is set.
pub const BLOCK_SIZE: i64 = 100 * 1024 * 1024;

/// Total size of every existing source file. Missing files contribute
/// zero — the caller already prints "File not found" notices when it
/// opens them.
#[must_use]
pub fn total_size(sources: &[StreamSource]) -> i64 {
	let mut total = 0i64;
	for s in sources {
		if let Ok(meta) = std::fs::metadata(&s.path) {
			total += meta.len() as i64;
		}
	}
	total
}

/// Render a byte count as a human-readable binary size string.
#[must_use]
pub fn format_bytes(n: i64) -> String {
	const KIB: i64 = 1024;
	const MIB: i64 = KIB * 1024;
	const GIB: i64 = MIB * 1024;
	if n >= GIB {
		format!("{:.1} GiB", n as f64 / GIB as f64)
	} else if n >= MIB {
		format!("{:.1} MiB", n as f64 / MIB as f64)
	} else if n >= KIB {
		format!("{:.1} KiB", n as f64 / KIB as f64)
	} else {
		format!("{n} B")
	}
}

/// Apply the 10/100 MiB policy. `yes` skips the prompt. `in` is the
/// reader used for the y/N answer (`std::io::stdin` in production,
/// substitutable in tests). Returns `Ok(())` when the read may proceed.
pub fn guard_large_read<R: BufRead>(
	sources: &[StreamSource],
	yes: bool,
	mut in_: R,
	tty: bool,
) -> Result<(), String> {
	let total = total_size(sources);
	if total < WARN_SIZE {
		return Ok(());
	}
	let size = format_bytes(total);
	let suggestions = "  --tail N        last N lines\n  --since 1h      time window\n  --grep pattern  regex filter";

	if total >= BLOCK_SIZE {
		if !yes {
			return Err(format!(
				"log size {size} exceeds {}; pass --yes to override or narrow with:\n{suggestions}",
				format_bytes(BLOCK_SIZE)
			));
		}
		eprintln!("\x1b[33mwarning:\x1b[0m reading {size} of logs (--yes set)");
		return Ok(());
	}

	// 10–100 MiB: warn + confirm if interactive, proceed otherwise.
	if yes {
		return Ok(());
	}
	if !tty {
		eprintln!("\x1b[33mwarning:\x1b[0m reading {size} of logs (non-tty, proceeding)");
		return Ok(());
	}
	eprint!("log size {size}. options:\n{suggestions}\nproceed anyway? [y/N] ");
	let mut answer = String::new();
	if let Err(e) = in_.read_line(&mut answer) {
		if e.kind() != io::ErrorKind::UnexpectedEof {
			return Err(format!("read confirmation: {e}"));
		}
	}
	let answer = answer.trim().to_lowercase();
	if answer != "y" && answer != "yes" {
		return Err("aborted by user".into());
	}
	Ok(())
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::fs::OpenOptions;
	use std::io::{BufReader, Cursor};
	use std::path::Path;

	fn source(path: &Path, label: &str) -> StreamSource {
		StreamSource::new(path.to_string_lossy().into_owned(), label.to_string())
	}

	fn make_file(dir: &Path, name: &str, size: i64) -> std::path::PathBuf {
		let p = dir.join(name);
		let f = OpenOptions::new()
			.write(true)
			.create(true)
			.truncate(true)
			.open(&p)
			.unwrap();
		f.set_len(size as u64).unwrap();
		drop(f);
		p
	}

	#[test]
	fn format_bytes_basic() {
		assert_eq!(format_bytes(0), "0 B");
		assert_eq!(format_bytes(500), "500 B");
		assert_eq!(format_bytes(1023), "1023 B");
		assert_eq!(format_bytes(2 * 1024), "2.0 KiB");
		assert_eq!(format_bytes(5 * 1024 * 1024), "5.0 MiB");
		assert_eq!(format_bytes(3 * 1024 * 1024 * 1024), "3.0 GiB");
	}

	#[test]
	fn format_bytes_lock_thresholds() {
		// Boundary cases the table module relies on.
		assert_eq!(format_bytes(WARN_SIZE), "10.0 MiB");
		assert_eq!(format_bytes(BLOCK_SIZE), "100.0 MiB");
	}

	#[test]
	fn total_size_zero_for_missing_files() {
		let dir = tempfile::tempdir().unwrap();
		let srcs = vec![
			source(&dir.path().join("missing1.log"), "STDOUT"),
			source(&dir.path().join("missing2.log"), "STDERR"),
		];
		assert_eq!(total_size(&srcs), 0);
	}

	#[test]
	fn total_size_sums_existing_files() {
		let dir = tempfile::tempdir().unwrap();
		let a = make_file(dir.path(), "a.log", 100);
		let b = make_file(dir.path(), "b.log", 200);
		let srcs = vec![source(&a, "STDOUT"), source(&b, "STDERR")];
		assert_eq!(total_size(&srcs), 300);
	}

	#[test]
	fn guard_below_threshold_no_op() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "small.log", 100);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b""));
		guard_large_read(&srcs, false, r, false).unwrap();
	}

	#[test]
	fn guard_block_without_yes() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "huge.log", BLOCK_SIZE + 1);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b""));
		let err = guard_large_read(&srcs, false, r, false).unwrap_err();
		assert!(err.contains("exceeds"), "unexpected: {err}");
	}

	#[test]
	fn guard_block_with_yes() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "huge.log", BLOCK_SIZE + 1);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b""));
		guard_large_read(&srcs, true, r, false).unwrap();
	}

	#[test]
	fn guard_warn_range_yes_skips_prompt() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "mid.log", WARN_SIZE + 1);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b""));
		guard_large_read(&srcs, true, r, false).unwrap();
	}

	#[test]
	fn guard_warn_range_non_tty_proceeds() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "mid.log", WARN_SIZE + 1);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b""));
		guard_large_read(&srcs, false, r, false).unwrap();
	}

	#[test]
	fn guard_warn_range_tty_yes_proceeds() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "mid.log", WARN_SIZE + 1);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b"y\n"));
		guard_large_read(&srcs, false, r, true).unwrap();
	}

	#[test]
	fn guard_warn_range_tty_no_aborts() {
		let dir = tempfile::tempdir().unwrap();
		let p = make_file(dir.path(), "mid.log", WARN_SIZE + 1);
		let srcs = vec![source(&p, "STDOUT")];
		let r = BufReader::new(Cursor::new(b"n\n"));
		let err = guard_large_read(&srcs, false, r, true).unwrap_err();
		assert!(err.contains("aborted"), "unexpected: {err}");
	}

	#[test]
	fn guard_block_missing_files_pass_quietly() {
		let dir = tempfile::tempdir().unwrap();
		let srcs = vec![
			source(&dir.path().join("missing1.log"), "STDOUT"),
			source(&dir.path().join("missing2.log"), "STDERR"),
		];
		let r = BufReader::new(Cursor::new(b""));
		guard_large_read(&srcs, false, r, false).unwrap();
	}
}
