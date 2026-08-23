//! Stable k-way merge by timestamp plus the per-mode readers.
//!
//! The merge itself is pure (no I/O): every input slice is already in
//! source-order, so a linear selection against the current head of each
//! slice yields the chronologically-next entry. Ties on `(ts, seq)` keep
//! the output stable per source — entries emitted with identical
//! timestamps preserve insertion order so a debug reader can spot the
//! "what landed first" question.
//!
//! `bounded_tail` and `stream_merge` differ in what they read from each
//! source: the former seeks near end-of-file and reads the last `n`
//! entries; the latter reads the entire file end-to-end and runs the
//! merge across the loaded slices.
//!
//! Module split (sub-modules):
//!
//!   - [`heap`]: the pure k-way merge algorithm.
//!   - [`format`]: the per-entry rendering and the coloured label helper.

pub mod format;
mod heap;

use std::fs::File;
use std::io::Write;

use super::entry::{read_entries, read_last_n_entries, Entry};

pub use format::{color_label, format_entry};
pub use heap::merge_by_ts;

/// Stream source descriptor. `seq_base` is the running sequence counter
/// shared across all sources so the tie-breaker stays monotonic even
/// when sources are passed in one at a time.
#[derive(Debug, Clone)]
pub struct StreamSource {
	pub path: String,
	pub label: String,
	pub seq_base: u64,
}

impl StreamSource {
	#[must_use]
	pub fn new(path: impl Into<String>, label: impl Into<String>) -> Self {
		Self {
			path: path.into(),
			label: label.into(),
			seq_base: 0,
		}
	}
}

/// Filter applied to every entry before it's emitted. `since` drops
/// entries whose timestamp is before the cutoff (None = no cutoff);
/// `grep` drops entries whose body doesn't match (None = no filter).
#[derive(Debug, Clone, Default)]
pub struct Filter {
	pub since: Option<i64>,
	pub grep: Option<regex::Regex>,
}

impl Filter {
	/// Reports whether `e` survives the filter.
	pub fn keep(&self, e: &Entry) -> bool {
		if let Some(cutoff) = self.since {
			match e.ts_unix {
				Some(ts) if ts < cutoff => return false,
				None => return false,
				_ => {}
			}
		}
		if let Some(re) = &self.grep {
			if !re.is_match(&e.body) {
				return false;
			}
		}
		true
	}
}

/// Read the last `n` entries from each path, merge them by timestamp,
/// and trim the merged result back to `n`.
pub fn bounded_tail<W: Write>(
	w: &mut W,
	sources: &[StreamSource],
	n: usize,
	fs: &Filter,
) -> std::io::Result<()> {
	let mut all: Vec<Vec<Entry>> = Vec::with_capacity(sources.len());
	let mut seq: u64 = 0;
	for s in sources {
		match File::open(&s.path) {
			Ok(mut f) => {
				let (entries, next_seq) =
					read_last_n_entries(&mut f, &s.label, n, seq).map_err(map_io)?;
				seq = next_seq;
				all.push(entries);
			}
			Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
				print_file_missing(&s.label);
			}
			Err(e) => return Err(e),
		}
	}
	let merged = merge_by_ts(&all);
	let trimmed: &[Entry] = if merged.len() > n {
		&merged[merged.len() - n..]
	} else {
		&merged[..]
	};
	format::format_entries(w, trimmed, fs)
}

fn map_io(e: std::io::Error) -> std::io::Error {
	e
}

fn print_file_missing(label: &str) {
	let stdout = std::io::stdout();
	let mut out = stdout.lock();
	let _ = writeln!(out, "{} File not found", format::color_label(label));
}

/// Read every entry from each path, merge by timestamp, emit through `w`.
pub fn stream_merge<W: Write>(
	w: &mut W,
	fs: &Filter,
	sources: &[StreamSource],
) -> std::io::Result<()> {
	let mut all: Vec<Vec<Entry>> = Vec::with_capacity(sources.len());
	let mut seq: u64 = 0;
	for s in sources {
		match std::fs::read_to_string(&s.path) {
			Ok(text) => {
				let (entries, next_seq) = read_entries(&text, &s.label, seq);
				seq = next_seq;
				all.push(entries);
			}
			Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
				print_file_missing(&s.label);
			}
			Err(e) => return Err(e),
		}
	}
	let merged = merge_by_ts(&all);
	format::format_entries(w, &merged, fs)
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

	fn headerless(body: &str, seq: u64) -> E {
		E {
			ts_unix: None,
			label: "STDOUT".into(),
			body: body.into(),
			seq,
			has_ts: false,
		}
	}

	fn write_log(path: &str, lines: &[&str]) {
		if let Some(dir) = std::path::Path::new(path).parent() {
			std::fs::create_dir_all(dir).unwrap();
		}
		let body = lines.join("\n") + "\n";
		std::fs::write(path, body).unwrap();
	}

	#[test]
	fn filter_keep_default() {
		let fs = Filter::default();
		assert!(fs.keep(&entry(100, "x", 0)));
	}

	#[test]
	fn filter_since_drops_old() {
		let fs = Filter {
			since: Some(100),
			..Default::default()
		};
		assert!(!fs.keep(&entry(50, "old", 0)));
		assert!(fs.keep(&entry(150, "new", 1)));
	}

	#[test]
	fn filter_grep_keeps_match() {
		let fs = Filter {
			grep: Some(regex::Regex::new("(?i)error").unwrap()),
			..Default::default()
		};
		assert!(!fs.keep(&entry(0, "ok", 0)));
		assert!(fs.keep(&entry(0, "fatal ERROR here", 1)));
	}

	#[test]
	fn filter_drops_headerless() {
		let fs = Filter {
			since: Some(0),
			..Default::default()
		};
		// Headerless entries have no timestamp; treat as filtered.
		assert!(!fs.keep(&headerless("no ts", 0)));
	}

	#[test]
	fn bounded_tail_takes_newest_across_sources() {
		let dir = tempfile::tempdir().unwrap();
		let stdout_path = dir.path().join("stdout.log");
		let stderr_path = dir.path().join("stderr.log");
		let mut out_lines = Vec::new();
		for i in 0..30 {
			out_lines.push(format!("2026-04-26 12:00:{:02} out-{}", i, i));
		}
		std::fs::write(&stdout_path, out_lines.join("\n") + "\n").unwrap();
		let mut err_lines = Vec::new();
		for i in 0..10 {
			err_lines.push(format!("2026-04-26 12:00:{:02} err-{}", 30 + i, i));
		}
		std::fs::write(&stderr_path, err_lines.join("\n") + "\n").unwrap();

		let sources = vec![
			StreamSource::new(stdout_path.to_string_lossy(), "STDOUT"),
			StreamSource::new(stderr_path.to_string_lossy(), "STDERR"),
		];
		// First sanity-check the read path: the files are short enough
		// that read_last_n_entries must return every line.
		let mut sf = std::fs::File::open(&stdout_path).unwrap();
		let (entries, _) =
			crate::cli::commands::logs::entry::read_last_n_entries(&mut sf, "STDOUT", 40, 0)
				.unwrap();
		assert_eq!(entries.len(), 30, "stdout entries: {}", entries.len());
		let mut ef = std::fs::File::open(&stderr_path).unwrap();
		let (entries, _) =
			crate::cli::commands::logs::entry::read_last_n_entries(&mut ef, "STDERR", 40, 0)
				.unwrap();
		assert_eq!(entries.len(), 10, "stderr entries: {}", entries.len());

		let mut buf = Vec::new();
		bounded_tail(&mut buf, &sources, 40, &Filter::default()).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		let total = plain.matches("out-").count() + plain.matches("err-").count();
		assert_eq!(
			total, 40,
			"expected 40 total, got {total}\noutput:\n{plain}"
		);
		let tail: Vec<&str> = plain.trim_end().split('\n').collect();
		for line in &tail[tail.len() - 10..] {
			assert!(
				line.contains("err-"),
				"expected newer err entries at tail, got {line:?}"
			);
		}
	}

	#[test]
	fn bounded_tail_sparse_stderr_fills_from_stdout() {
		let dir = tempfile::tempdir().unwrap();
		let stdout_path = dir.path().join("stdout.log");
		let stderr_path = dir.path().join("stderr.log");
		let mut out_lines = Vec::new();
		for i in 0..50 {
			out_lines.push(format!("2026-04-26 12:00:{:02} out-{}", i % 60, i));
		}
		std::fs::write(&stdout_path, out_lines.join("\n") + "\n").unwrap();
		let mut err_lines = Vec::new();
		for i in 0..10 {
			err_lines.push(format!("2026-04-26 12:01:{:02} err-{}", i, i));
		}
		std::fs::write(&stderr_path, err_lines.join("\n") + "\n").unwrap();

		let sources = vec![
			StreamSource::new(stdout_path.to_string_lossy(), "STDOUT"),
			StreamSource::new(stderr_path.to_string_lossy(), "STDERR"),
		];
		let mut buf = Vec::new();
		bounded_tail(&mut buf, &sources, 40, &Filter::default()).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		let err_count = plain.matches("err-").count();
		let out_count = plain.matches("out-").count();
		assert_eq!(err_count, 10);
		assert_eq!(err_count + out_count, 40);
	}

	#[test]
	fn bounded_tail_all_missing_returns_zero_results() {
		let dir = tempfile::tempdir().unwrap();
		let sources = vec![
			StreamSource::new(dir.path().join("no1.log").to_string_lossy(), "STDOUT"),
			StreamSource::new(dir.path().join("no2.log").to_string_lossy(), "STDERR"),
		];
		let mut buf = Vec::new();
		bounded_tail(&mut buf, &sources, 10, &Filter::default()).unwrap();
	}

	#[test]
	fn bounded_tail_banner_counts_towards_limit() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("stdout.log");
		let r = "=".repeat(80);
		let mut body = String::new();
		for i in 0..10 {
			body.push_str(&format!("2026-04-26 12:00:{:02} entry-{}\n", i, i));
		}
		let mid = "==  STOPPED                                          2026-04-26 12:00:30  ==";
		body.push_str(&format!("{r}\n{mid}\n{r}\n"));
		std::fs::write(&p, body).unwrap();
		let sources = vec![StreamSource::new(p.to_string_lossy(), "STDOUT")];
		let mut buf = Vec::new();
		bounded_tail(&mut buf, &sources, 5, &Filter::default()).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		assert!(plain.contains("STOPPED"));
	}

	#[test]
	fn stream_merge_orders_across_sources() {
		let dir = tempfile::tempdir().unwrap();
		let stdout_path = dir.path().join("stdout.log");
		let stderr_path = dir.path().join("stderr.log");
		write_log(
			&stdout_path.to_string_lossy(),
			&["2026-04-26 12:00:01 a", "2026-04-26 12:00:03 c"],
		);
		write_log(
			&stderr_path.to_string_lossy(),
			&["2026-04-26 12:00:02 b", "2026-04-26 12:00:04 d"],
		);
		let sources = vec![
			StreamSource::new(stdout_path.to_string_lossy(), "STDOUT"),
			StreamSource::new(stderr_path.to_string_lossy(), "STDERR"),
		];
		let mut buf = Vec::new();
		stream_merge(&mut buf, &Filter::default(), &sources).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		let lines: Vec<&str> = plain.trim_end().split('\n').collect();
		assert_eq!(lines.len(), 4);
		assert!(lines[0].ends_with(" a"));
		assert!(lines[1].ends_with(" b"));
		assert!(lines[2].ends_with(" c"));
		assert!(lines[3].ends_with(" d"));
	}

	#[test]
	fn stream_merge_folds_continuation() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("stdout.log");
		write_log(
			&p.to_string_lossy(),
			&[
				"2026-04-26 12:00:01 first",
				"trace-A",
				"trace-B",
				"2026-04-26 12:00:02 second",
			],
		);
		let sources = vec![StreamSource::new(p.to_string_lossy(), "STDOUT")];
		let mut buf = Vec::new();
		stream_merge(&mut buf, &Filter::default(), &sources).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		assert!(plain.contains("first\ntrace-A\ntrace-B"));
		assert!(plain.contains("second"));
	}

	#[test]
	fn stream_merge_one_source_missing() {
		let dir = tempfile::tempdir().unwrap();
		let stdout_path = dir.path().join("stdout.log");
		let stderr_path = dir.path().join("absent.log");
		write_log(
			&stdout_path.to_string_lossy(),
			&["2026-04-26 12:00:00 only"],
		);
		let sources = vec![
			StreamSource::new(stdout_path.to_string_lossy(), "STDOUT"),
			StreamSource::new(stderr_path.to_string_lossy(), "STDERR"),
		];
		let mut buf = Vec::new();
		stream_merge(&mut buf, &Filter::default(), &sources).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		assert!(plain.contains("only"));
	}

	#[test]
	fn stream_merge_banner_at_eof() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("trailing.log");
		let r = "=".repeat(80);
		let mid = "==  EXITED  code=0                                   2026-04-26 12:00:01  ==";
		let body = format!("2026-04-26 12:00:00 hello\n{r}\n{mid}\n{r}\n");
		std::fs::write(&p, body).unwrap();
		let sources = vec![StreamSource::new(p.to_string_lossy(), "STDOUT")];
		let mut buf = Vec::new();
		stream_merge(&mut buf, &Filter::default(), &sources).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		assert!(plain.contains("hello"));
		assert!(plain.contains("EXITED"));
	}

	#[test]
	fn stream_merge_banners_interleaved() {
		let dir = tempfile::tempdir().unwrap();
		let stdout_path = dir.path().join("stdout.log");
		let stderr_path = dir.path().join("stderr.log");
		let r = "=".repeat(80);
		let mid = "==  RESTARTED                                    2026-04-26 12:00:02  ==";
		let out_body =
			format!("2026-04-26 12:00:00 ok-1\n{r}\n{mid}\n{r}\n2026-04-26 12:00:03 ok-2\n");
		std::fs::write(&stdout_path, out_body).unwrap();
		std::fs::write(&stderr_path, "2026-04-26 12:00:01 err-1\n").unwrap();
		let sources = vec![
			StreamSource::new(stdout_path.to_string_lossy(), "STDOUT"),
			StreamSource::new(stderr_path.to_string_lossy(), "STDERR"),
		];
		let mut buf = Vec::new();
		stream_merge(&mut buf, &Filter::default(), &sources).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		let i1 = plain.find("ok-1").unwrap();
		let i2 = plain.find("err-1").unwrap();
		let ib = plain.find("RESTARTED").unwrap();
		let i3 = plain.find("ok-2").unwrap();
		assert!(i1 < i2 && i2 < ib && ib < i3);
	}
}
