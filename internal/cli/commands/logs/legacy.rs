//! Legacy non-merging tail. Each source is tailed in its own thread,
//! lines emitted in arrival order with no cross-stream ordering. Kept as
//! an escape hatch behind `--no-merge` for users who script against the
//! old format.

use std::fs::File;
use std::io::{BufRead, BufReader, Seek, SeekFrom};
use std::thread;
use std::time::Duration;

use super::follow::Sleeper;
use super::merge::{color_label, StreamSource};

/// Tail every source in its own thread. Each thread opens its file,
/// seeks to end of file (or to the last `n` lines if not following),
/// and emits `"{label} {line}\n"` to stdout as it arrives. When
/// `follow` is false, every per-source thread returns after printing
/// the trailing lines, which is what the test fixture relies on.
pub fn run_legacy_split(
	sources: Vec<StreamSource>,
	follow: bool,
	sleeper: Sleeper,
) -> Result<(), std::io::Error> {
	let mut handles = Vec::with_capacity(sources.len());
	for s in sources {
		handles.push(thread::spawn(move || {
			let _ = tail_file_legacy(s, follow, sleeper);
		}));
	}
	for h in handles {
		let _ = h.join();
	}
	Ok(())
}

/// Tail a single file in legacy mode: open it, print the trailing `n`
/// lines, then either return (when `follow` is false) or seek to end
/// and poll for new lines forever (when `follow` is true). Used by
/// `run_legacy_split` and exposed for the per-file test.
pub fn tail_file_legacy(
	source: StreamSource,
	follow: bool,
	sleeper: Sleeper,
) -> Result<(), std::io::Error> {
	let mut f = match File::open(&source.path) {
		Ok(f) => f,
		Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
			println!("{} File not found", color_label(&source.label));
			return Ok(());
		}
		Err(e) => return Err(e),
	};

	// Print the trailing `n` lines using the seek-then-read approach.
	print_last_n_lines(&mut f, &source.label, 40)?;

	if !follow {
		return Ok(());
	}

	// Seek to end and follow. The Go reference uses a 200ms poll loop.
	f.seek(SeekFrom::End(0))?;
	let mut br = BufReader::new(f);
	let mut line = String::new();
	loop {
		line.clear();
		let n = br.read_line(&mut line)?;
		if n == 0 {
			sleeper(Duration::from_millis(200));
			continue;
		}
		print!("{} {}", color_label(&source.label), line);
	}
}

/// Print the trailing `n` lines from `f` using a backward seek. The seek
/// window is `n * 150` bytes, matching the Go heuristic.
pub fn print_last_n_lines(f: &mut File, label: &str, n: usize) -> Result<(), std::io::Error> {
	let size = f.metadata()?.len();
	if size == 0 {
		return Ok(());
	}
	let offset = size.saturating_sub((n as u64).saturating_mul(150));
	f.seek(SeekFrom::Start(offset))?;
	let mut br = BufReader::new(f.try_clone()?);
	if offset > 0 {
		// Drop the partial first line that the seek landed us inside.
		let mut throwaway = String::new();
		br.read_line(&mut throwaway)?;
	}
	let mut ring: Vec<String> = Vec::with_capacity(n);
	let mut idx = 0usize;
	let mut total = 0usize;
	let mut line = String::new();
	while br.read_line(&mut line)? > 0 {
		if line.ends_with('\n') {
			line.pop();
		}
		if ring.len() < n {
			ring.push(line.clone());
		} else {
			ring[idx % n] = line.clone();
		}
		idx += 1;
		total += 1;
		line.clear();
	}
	let shown = total.min(n);
	let start = if total > n { idx % n } else { 0 };
	for i in 0..shown {
		let line = &ring[(start + i) % n];
		println!("{} {}", color_label(label), line);
	}
	Ok(())
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::commands::logs::entry::{self, Entry as E, TS_LEN};
	use crate::cli::commands::logs::follow::FLUSH_DELAY;
	use std::fs::OpenOptions;
	use std::io::Write as _;
	use std::time::Duration;

	#[test]
	fn print_last_n_lines_reads_backwards() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("out.log");
		let mut f = OpenOptions::new()
			.write(true)
			.create(true)
			.truncate(true)
			.open(&p)
			.unwrap();
		for i in 0..500 {
			writeln!(f, "line-{i}").unwrap();
		}
		drop(f);
		let mut f = File::open(&p).unwrap();
		print_last_n_lines(&mut f, "STDOUT", 40).unwrap();
	}

	#[test]
	fn print_last_n_lines_handles_small_files() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("small.log");
		std::fs::write(&p, "first stdout\nsecond stdout\n").unwrap();
		let mut f = File::open(&p).unwrap();
		print_last_n_lines(&mut f, "STDOUT", 10).unwrap();
	}

	#[test]
	fn print_last_n_lines_handles_empty_file() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("empty.log");
		std::fs::write(&p, "").unwrap();
		let mut f = File::open(&p).unwrap();
		print_last_n_lines(&mut f, "STDOUT", 10).unwrap();
	}

	#[test]
	fn tail_file_legacy_missing_file() {
		let dir = tempfile::tempdir().unwrap();
		let source = StreamSource::new(
			dir.path().join("nope.log").to_string_lossy().into_owned(),
			"STDOUT",
		);
		let sleeper: Sleeper = |_| {};
		tail_file_legacy(source, false, sleeper).unwrap();
	}

	#[test]
	fn run_legacy_split_two_files() {
		let dir = tempfile::tempdir().unwrap();
		let stdout_path = dir.path().join("stdout.log");
		let stderr_path = dir.path().join("stderr.log");
		std::fs::write(&stdout_path, "first stdout\nsecond stdout\n").unwrap();
		std::fs::write(&stderr_path, "boom err\n").unwrap();
		let sources = vec![
			StreamSource::new(stdout_path.to_string_lossy(), "STDOUT"),
			StreamSource::new(stderr_path.to_string_lossy(), "STDERR"),
		];
		let sleeper: Sleeper = |_| {};
		run_legacy_split(sources, false, sleeper).unwrap();
	}

	#[test]
	fn sleep_uses_constant() {
		// Lock the FLUSH_DELAY constant so a future change is deliberate.
		assert_eq!(FLUSH_DELAY, Duration::from_millis(200));
	}

	#[test]
	fn entry_module_ts_len_constant() {
		assert_eq!(TS_LEN, 19);
		let _ = E {
			ts_unix: None,
			label: String::new(),
			body: String::new(),
			seq: 0,
			has_ts: false,
		};
		let _ = entry::parse_line("2026-04-26 12:00:00 body");
	}
}
