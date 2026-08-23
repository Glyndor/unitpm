//! Log entry parsing — `parseLine`, banner detection, batch readers.
//!
//! Mirrors `internal/cli/commands/logs/merge.go` for the entry-level
//! concerns: every other module in the package consumes [`Entry`] values
//! produced here. The 3-line banner block (rule / middle / rule) is the
//! lifecycle event marker written by the daemon; this module recognises it
//! and folds continuation lines into the prior timestamped entry.

use std::fs::File;
use std::io::{BufRead, BufReader, Read, Seek, SeekFrom};

use crate::cli::format::{format_unix_local, now_unix_seconds};

/// Width of the timestamp prefix: `"YYYY-MM-DD HH:MM:SS "`.
pub const TS_LEN: usize = 19;

/// One chronologically-anchored log record. Multi-line bodies (banners,
/// stack traces) fold under one anchor ts.
#[derive(Debug, Clone, PartialEq)]
pub struct Entry {
	pub ts_unix: Option<i64>,
	pub label: String,
	pub body: String,
	/// Sequence number per source — used as a tie-breaker when two sources
	/// emit entries with identical timestamps so the merge stays stable.
	pub seq: u64,
	/// True for entries with a parseable timestamp; controls the rendered
	/// placeholder for header-less lines at file head.
	pub has_ts: bool,
}

/// Local-time timestamp. We only need unix-seconds for ordering; the
/// formatted form is used only when rendering an entry back to the user,
/// and that goes through [`crate::cli::format::time::format_unix_local`]
/// so the two stay in sync.
fn parse_local_timestamp(s: &str) -> Option<i64> {
	// "YYYY-MM-DD HH:MM:SS" parsed as the local civil time and turned
	// into unix seconds. We delegate to libc so the `TZ` environment
	// variable works the same way the Go `time.Local` does.
	let bytes = s.as_bytes();
	if bytes.len() < TS_LEN {
		return None;
	}
	let year: i32 = s[..4].parse().ok()?;
	let month: u32 = s[5..7].parse().ok()?;
	let day: u32 = s[8..10].parse().ok()?;
	let hour: u32 = s[11..13].parse().ok()?;
	let minute: u32 = s[14..16].parse().ok()?;
	let second: u32 = s[17..19].parse().ok()?;
	if !(1..=12).contains(&month) || day < 1 || hour > 23 || minute > 59 || second > 59 {
		return None;
	}
	let days = days_from_civil(year, month, day)?;
	let naive = days * 86_400 + (hour as i64) * 3_600 + (minute as i64) * 60 + second as i64;
	// Convert "local civil time" to actual unix seconds by subtracting
	// the timezone offset. Probe by treating the naive value as if it
	// were UTC and asking libc to convert it back. The DST transition
	// day can flip the offset, so re-probe after the first correction.
	let first = localtime_offset(naive);
	let second = localtime_offset(naive - first);
	Some(naive - second)
}

fn days_from_civil(y: i32, m: u32, d: u32) -> Option<i64> {
	let y = if m <= 2 { y - 1 } else { y };
	let era: i64 = if y >= 0 { y as i64 } else { y as i64 - 399 } / 400;
	// `era` here is the number of completed 400-year cycles since
	// 0000-03-01. The original Howard Hinnant algorithm uses
	// `(y - era * 400)` to recover the year-of-era, so `era` must be
	// `y / 400` (not `y` itself).
	let yoe: i64 = (y as i64) - era * 400;
	let m_u: i64 = if m > 2 { m as i64 - 3 } else { m as i64 + 9 };
	let d_i: i64 = d as i64;
	let doy: i64 = (153 * m_u + 2) / 5 + d_i - 1;
	let doe: i64 = yoe * 365 + yoe / 4 - yoe / 100 + doy;
	era.checked_mul(146_097)?
		.checked_add(doe)?
		.checked_sub(719_468)
}

fn localtime_offset(secs: i64) -> i64 {
	#[repr(C)]
	struct Tm {
		tm_sec: i32,
		tm_min: i32,
		tm_hour: i32,
		tm_mday: i32,
		tm_mon: i32,
		tm_year: i32,
		tm_wday: i32,
		tm_yday: i32,
		tm_isdst: i32,
		tm_gmtoff: i64,
		tm_zone: *const i8,
	}
	let mut tm = Tm {
		tm_sec: 0,
		tm_min: 0,
		tm_hour: 0,
		tm_mday: 0,
		tm_mon: 0,
		tm_year: 0,
		tm_wday: 0,
		tm_yday: 0,
		tm_isdst: 0,
		tm_gmtoff: 0,
		tm_zone: std::ptr::null(),
	};
	let rc =
		unsafe { libc::localtime_r(&(secs as libc::time_t), &mut tm as *mut Tm as *mut libc::tm) };
	if rc.is_null() {
		return 0;
	}
	tm.tm_gmtoff
}

/// Parse a single line. Returns `(ts, body, ok)`. `ok=false` means the
/// line has no parseable timestamp — caller should fold it into the prior
/// entry or drop it at file head.
pub fn parse_line(line: &str) -> (Option<i64>, String, bool) {
	let bytes = line.as_bytes();
	if bytes.len() < TS_LEN + 1 {
		return (None, line.to_string(), false);
	}
	let ts_str = match std::str::from_utf8(&bytes[..TS_LEN]) {
		Ok(s) => s,
		Err(_) => return (None, line.to_string(), false),
	};
	let ts_unix = match parse_local_timestamp(ts_str) {
		Some(t) => t,
		None => return (None, line.to_string(), false),
	};
	let body = &line[TS_LEN..];
	let trimmed = body.trim_start();
	(Some(ts_unix), trimmed.to_string(), true)
}

/// Reports whether a line is the top/bottom rule of a lifecycle banner —
/// a non-empty run of `=` chars. The daemon uses an 80-char run; we accept
/// any length ≥8 to stay robust against future width changes.
pub fn is_banner_rule(line: &str) -> bool {
	if line.len() < 8 {
		return false;
	}
	line.bytes().all(|b| b == b'=')
}

/// Decode the middle line of a banner: `==  EVENT  ==...==  YYYY-MM-DD HH:MM:SS  ==`.
/// The trailing 4 chars are always `"  =="` and the 19 chars before that are
/// the timestamp.
pub fn parse_banner_middle(line: &str) -> Option<i64> {
	const TAIL: &str = "  ==";
	if !line.ends_with(TAIL) {
		return None;
	}
	if line.len() < TAIL.len() + TS_LEN {
		return None;
	}
	let inner = &line[..line.len() - TAIL.len()];
	let ts_str = &inner[inner.len() - TS_LEN..];
	parse_local_timestamp(ts_str)
}

/// Inspect three consecutive lines and, if they form a lifecycle banner,
/// return the synthesized entry. `None` falls through to the regular
/// ts-line path.
pub fn try_consume_banner(
	rule1: &str,
	mid: &str,
	rule2: &str,
	label: &str,
	seq: u64,
) -> Option<Entry> {
	if !is_banner_rule(rule1) || !is_banner_rule(rule2) {
		return None;
	}
	let ts_unix = parse_banner_middle(mid)?;
	let body = format!("{rule1}\n{mid}\n{rule2}");
	Some(Entry {
		ts_unix: Some(ts_unix),
		label: label.to_string(),
		body,
		has_ts: true,
		seq,
	})
}

/// Read all entries from `input`. Continuation lines fold into the prior
/// entry. Lifecycle banners (3-line `===` / middle / `===` blocks) become
/// a standalone entry with the timestamp embedded in the middle line.
/// Returns the next seq value so multiple sources can share a monotonic
/// counter.
pub fn read_entries(input: &str, label: &str, start_seq: u64) -> (Vec<Entry>, u64) {
	let mut out: Vec<Entry> = Vec::new();
	let mut seq = start_seq;
	let lines: Vec<&str> = input.split('\n').collect();
	let mut i = 0;
	while i < lines.len() {
		let line = lines[i];
		if line.is_empty() {
			i += 1;
			continue;
		}
		if is_banner_rule(line) && i + 2 < lines.len() {
			if let Some(e) = try_consume_banner(lines[i], lines[i + 1], lines[i + 2], label, seq) {
				out.push(e);
				seq += 1;
				i += 3;
				continue;
			}
		}
		let (ts, body, ok) = parse_line(line);
		if ok {
			out.push(Entry {
				ts_unix: ts,
				label: label.to_string(),
				body,
				has_ts: true,
				seq,
			});
			seq += 1;
		} else if let Some(last) = out.last_mut() {
			last.body.push('\n');
			last.body.push_str(line);
		} else {
			out.push(Entry {
				ts_unix: None,
				label: label.to_string(),
				body: line.to_string(),
				has_ts: false,
				seq,
			});
			seq += 1;
		}
		i += 1;
	}
	(out, seq)
}

/// Seek near the end of `f` and read at most `n` entries. The seek
/// window grows if too few entries are recovered (e.g. very long lines),
/// bounded so we never scan more than the whole file.
pub fn read_last_n_entries(
	f: &mut File,
	label: &str,
	n: usize,
	start_seq: u64,
) -> Result<(Vec<Entry>, u64), std::io::Error> {
	let size = f.metadata()?.len();
	if size == 0 {
		return Ok((Vec::new(), start_seq));
	}

	let mut guess: u64 = (n as u64).saturating_mul(200);
	let mut seq = start_seq;
	let mut buf = String::new();
	for _attempt in 0..4u64 {
		let target = if guess > size { size } else { guess };
		f.seek(SeekFrom::Start(size - target))?;
		let mut br = BufReader::new(f.try_clone()?);
		if target < size {
			// Drop the partial first line.
			let mut throwaway = String::new();
			if br.read_line(&mut throwaway)? == 0 {
				return Ok((Vec::new(), seq));
			}
		}
		buf.clear();
		br.read_to_string(&mut buf)?;
		let (mut entries, next_seq) = read_entries(&buf, label, seq);
		seq = next_seq;
		if entries.len() >= n || guess >= size {
			if entries.len() > n {
				let drop = entries.len() - n;
				entries.drain(0..drop);
			}
			return Ok((entries, seq));
		}
		guess = guess.saturating_mul(4);
		if guess > size {
			guess = size;
		}
	}
	f.seek(SeekFrom::Start(0))?;
	let mut br = BufReader::new(f.try_clone()?);
	buf.clear();
	br.read_to_string(&mut buf)?;
	let (mut entries, next_seq) = read_entries(&buf, label, seq);
	seq = next_seq;
	if entries.len() > n {
		let drop = entries.len() - n;
		entries.drain(0..drop);
	}
	Ok((entries, seq))
}

/// Re-export of the formatter used by [`format_entry`](super::merge::format_entry)
/// — local-time rendering of an entry's timestamp.
pub fn render_ts(ts_unix: Option<i64>) -> String {
	match ts_unix {
		Some(t) => format_unix_local(t),
		None => " ".repeat(TS_LEN),
	}
}

/// Re-export of `now_unix_seconds` so callers don't reach across modules.
#[inline]
pub fn now_unix() -> i64 {
	now_unix_seconds()
}

#[cfg(test)]
mod tests {
	use super::*;

	fn parse(s: &str) -> (Option<i64>, String, bool) {
		parse_line(s)
	}

	#[test]
	fn parse_line_valid() {
		let (ts, body, ok) = parse("2026-04-26 12:00:00 hello");
		assert!(ok);
		assert_eq!(body, "hello");
		let ts = ts.unwrap();
		assert_eq!(format_unix_local(ts), "2026-04-26 12:00:00");
	}

	#[test]
	fn parse_line_strips_leading_space() {
		let (_, body, ok) = parse("2026-04-26 12:00:00  body");
		assert!(ok);
		assert_eq!(body, "body");
	}

	#[test]
	fn parse_line_no_body() {
		let (_, body, ok) = parse("2026-04-26 12:00:00 ");
		assert!(ok);
		assert_eq!(body, "");
	}

	#[test]
	fn parse_line_too_short() {
		let (_, _, ok) = parse("2026-04-26");
		assert!(!ok);
	}

	#[test]
	fn parse_line_bad_date() {
		let (_, _, ok) = parse("2026-99-99 99:99:99 oops");
		assert!(!ok);
	}

	#[test]
	fn parse_line_empty() {
		let (_, _, ok) = parse("");
		assert!(!ok);
	}

	#[test]
	fn parse_line_banner_equals() {
		let (_, _, ok) = parse("================================");
		assert!(!ok);
	}

	#[test]
	fn parse_line_short() {
		let (_, _, ok) = parse("short");
		assert!(!ok);
	}

	#[test]
	fn is_banner_rule_lengths() {
		assert!(is_banner_rule(&"=".repeat(80)));
		assert!(is_banner_rule(&"=".repeat(8)));
		assert!(!is_banner_rule(&"=".repeat(7)));
		assert!(!is_banner_rule(""));
		assert!(!is_banner_rule("== STARTED =="));
		assert!(!is_banner_rule(&format!("={}", " ".repeat(80))));
	}

	#[test]
	fn parse_banner_middle_ok() {
		let mid =
			"==  STARTED                                              2026-04-26 12:00:00  ==";
		let ts = parse_banner_middle(mid).unwrap();
		assert_eq!(format_unix_local(ts), "2026-04-26 12:00:00");
	}

	#[test]
	fn parse_banner_middle_no_ts() {
		assert!(parse_banner_middle("==  STARTED  ==").is_none());
	}

	#[test]
	fn parse_banner_middle_empty() {
		assert!(parse_banner_middle("").is_none());
	}

	#[test]
	fn read_entries_folds_continuation() {
		let input = "2026-04-26 12:00:00 first line\ncontinuation A\ncontinuation B\n2026-04-26 12:00:01 second line\n";
		let (entries, _) = read_entries(input, "STDOUT", 0);
		assert_eq!(entries.len(), 2);
		assert!(entries[0].body.contains("continuation A"));
		assert!(entries[0].body.contains("continuation B"));
		assert_eq!(entries[1].body, "second line");
	}

	#[test]
	fn read_entries_banner_surfaces_as_entry() {
		let rule = "=".repeat(80);
		let mid = "==  STARTED                                          2026-04-26 12:00:00  ==";
		let input = format!(
			"2026-04-26 11:59:59 before\n{rule}\n{mid}\n{rule}\n2026-04-26 12:00:01 after\n"
		);
		let (entries, _) = read_entries(&input, "STDOUT", 0);
		assert_eq!(entries.len(), 3);
		assert!(entries[1].body.contains("STARTED"));
		assert_eq!(
			format_unix_local(entries[1].ts_unix.unwrap()),
			"2026-04-26 12:00:00"
		);
		assert!(entries[0].ts_unix.unwrap() < entries[1].ts_unix.unwrap());
		assert!(entries[1].ts_unix.unwrap() < entries[2].ts_unix.unwrap());
	}

	#[test]
	fn read_entries_multiple_lifecycle_banners() {
		let r = "=".repeat(80);
		let input = format!(
			"{r}\n==  STARTED                                          2026-04-26 12:00:00  ==\n{r}\n\
			 2026-04-26 12:00:30 working\n\
			 {r}\n==  RESTARTED                                        2026-04-26 12:01:00  ==\n{r}\n\
			 2026-04-26 12:01:30 working again\n\
			 {r}\n==  STOPPED                                          2026-04-26 12:02:00  ==\n{r}\n"
		);
		let (entries, _) = read_entries(&input, "STDOUT", 0);
		assert_eq!(entries.len(), 5);
		let wants = [
			"STARTED",
			"working",
			"RESTARTED",
			"working again",
			"STOPPED",
		];
		for (i, want) in wants.iter().enumerate() {
			assert!(
				entries[i].body.contains(*want),
				"[{i}] body={}",
				entries[i].body
			);
		}
	}

	#[test]
	fn read_last_n_entries_tiny_file() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("tiny.log");
		std::fs::write(&p, "2026-04-26 12:00:00 a\n2026-04-26 12:00:01 b\n").unwrap();
		let mut f = File::open(&p).unwrap();
		let (entries, _) = read_last_n_entries(&mut f, "STDOUT", 100, 0).unwrap();
		assert_eq!(entries.len(), 2);
	}

	#[test]
	fn read_last_n_entries_empty() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("empty.log");
		std::fs::write(&p, "").unwrap();
		let mut f = File::open(&p).unwrap();
		let (entries, _) = read_last_n_entries(&mut f, "STDOUT", 10, 0).unwrap();
		assert_eq!(entries.len(), 0);
	}

	#[test]
	fn read_last_n_entries_smoke() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("big.log");
		let mut body = String::new();
		for i in 0..200 {
			body.push_str(&format!("2026-04-26 12:00:{:02} line-{}\n", i % 60, i));
		}
		std::fs::write(&p, body).unwrap();
		let mut f = File::open(&p).unwrap();
		let (entries, _) = read_last_n_entries(&mut f, "STDOUT", 30, 0).unwrap();
		assert_eq!(entries.len(), 30);
		assert!(entries.last().unwrap().body.ends_with("line-199"));
	}

	#[test]
	fn read_last_n_entries_long_lines_force_expansion() {
		let dir = tempfile::tempdir().unwrap();
		let p = dir.path().join("long.log");
		let long = "x".repeat(5000);
		let mut body = String::new();
		for i in 0..50 {
			body.push_str(&format!("2026-04-26 12:00:{:02} {}-{}\n", i % 60, long, i));
		}
		std::fs::write(&p, body).unwrap();
		let mut f = File::open(&p).unwrap();
		let (entries, _) = read_last_n_entries(&mut f, "STDOUT", 10, 0).unwrap();
		assert_eq!(entries.len(), 10);
		assert!(entries.last().unwrap().body.ends_with("-49"));
	}
}
