//! Shared human-readable formatters for the CLI.
//!
//! 10 cases ported from `internal/cli/format/format_test.go`.

mod time;

use std::time::Duration;

pub use time::{format_unix_local, now_unix_seconds, parse_rfc3339, relative_age};

/// Format a byte count using binary (1024) units: `"512 B"`, `"1.5 MB"`.
#[must_use]
pub fn bytes(b: i64) -> String {
	const UNIT: i64 = 1024;
	if b < UNIT {
		return format!("{b} B");
	}
	let mut div = UNIT;
	let mut exp = 0i64;
	let mut n = b / UNIT;
	while n >= UNIT {
		div *= UNIT;
		exp += 1;
		n /= UNIT;
	}
	format!(
		"{:.1} {}B",
		b as f64 / div as f64,
		"KMGTPE".as_bytes()[exp as usize] as char
	)
}

/// Format a byte count as both human-readable and raw bytes,
/// e.g. `"232.6 MB (243867648 bytes)"`. Values below 1 KiB skip the raw form.
#[must_use]
pub fn bytes_exact(b: i64) -> String {
	if b < 1024 {
		return bytes(b);
	}
	format!("{} ({} bytes)", bytes(b), b)
}

/// Render milliseconds as a compact duration string with at most two units:
/// `"22m 9s"`, `"2d 3h"`. Non-positive input renders as a dimmed `-`.
#[must_use]
pub fn uptime(ms: i64) -> String {
	if ms <= 0 {
		return crate::term::dim(format_args!("{}", "-"));
	}

	let d = Duration::from_millis(ms as u64);
	let total_secs = d.as_secs();
	let days = total_secs / 86_400;
	let hours = (total_secs / 3_600) % 24;
	let minutes = (total_secs / 60) % 60;
	let seconds = total_secs % 60;

	match () {
		_ if days > 0 && hours > 0 => format!("{days}d {hours}h"),
		_ if days > 0 => format!("{days}d"),
		_ if hours > 0 && minutes > 0 => format!("{hours}h {minutes}m"),
		_ if hours > 0 => format!("{hours}h"),
		_ if minutes > 0 && seconds > 0 => format!("{minutes}m {seconds}s"),
		_ if minutes > 0 => format!("{minutes}m"),
		_ => format!("{seconds}s"),
	}
}

/// Render milliseconds as both human form and raw ms,
/// e.g. `"22m 9s (1329123 ms)"`. Non-positive input renders as a dimmed `-`.
#[must_use]
pub fn uptime_exact(ms: i64) -> String {
	if ms <= 0 {
		return crate::term::dim(format_args!("{}", "-"));
	}
	format!("{} ({} ms)", uptime(ms), ms)
}

/// Format an RFC3339 (or RFC3339-nano) timestamp as
/// `"<abs> (<relative>)"`, e.g. `"2026-04-19 14:03:22 (2h ago)"`. Falls back
/// to the raw string on parse failure, or a dimmed `-` when empty.
#[must_use]
pub fn timestamp(ts: &str) -> String {
	if ts.is_empty() {
		return crate::term::dim(format_args!("{}", "-"));
	}

	let parsed = match parse_rfc3339(ts) {
		Some(t) => t,
		None => return ts.to_string(),
	};

	let delta = now_unix_seconds() - parsed;
	let abs = format_unix_local(parsed);
	format!("{abs} ({})", relative_age(delta))
}

/// Render a CPU/memory-like percentage. Zero renders as `"0%"` (no
/// decimal), everything else as `"%.1f%%"`.
#[must_use]
pub fn percent(v: f64) -> String {
	if v == 0.0 {
		return "0%".to_string();
	}
	format!("{v:.1}%")
}

/// Remove ANSI escape sequences from `s` so width calculations and
/// non-TTY output get a clean payload.
#[must_use]
pub fn strip_ansi(s: &str) -> String {
	let mut out = String::with_capacity(s.len());
	let mut in_seq = false;
	for ch in s.chars() {
		if ch == '\u{1b}' {
			in_seq = true;
			continue;
		}
		if in_seq {
			if ch.is_ascii_alphabetic() {
				in_seq = false;
			}
		} else {
			out.push(ch);
		}
	}
	out
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn bytes_human_readable() {
		let cases = [
			(0i64, "0 B"),
			(512, "512 B"),
			(1024, "1.0 KB"),
			(1024 * 1024, "1.0 MB"),
			(1024_i64.pow(3), "1.0 GB"),
		];
		for (input, want) in cases {
			let got = bytes(input);
			assert_eq!(got, want, "bytes({input})");
		}
	}

	#[test]
	fn bytes_exact_with_raw_form() {
		assert_eq!(bytes_exact(512), "512 B");
		let got = bytes_exact(1024 * 1024);
		assert!(got.contains("1.0 MB"), "expected MB form, got {got:?}");
		assert!(
			got.contains("1048576 bytes"),
			"expected raw byte count, got {got:?}"
		);
	}

	#[test]
	fn uptime_compact_two_units() {
		let cases = [
			(1000i64, "1s"),
			(61_000, "1m 1s"),
			(3_600_000, "1h"),
			(3_660_000, "1h 1m"),
			(86_400_000, "1d"),
		];
		for (input, want) in cases {
			let got = uptime(input);
			assert_eq!(got, want, "uptime({input})");
		}
	}

	#[test]
	fn percent_zero_and_fractional() {
		assert_eq!(percent(0.0), "0%");
		assert_eq!(percent(1.5), "1.5%");
	}

	#[test]
	fn strip_ansi_removes_color_codes() {
		assert_eq!(strip_ansi("\u{1b}[31mhello\u{1b}[0m"), "hello");
	}

	#[test]
	fn timestamp_empty_returns_dim_dash() {
		let got = timestamp("");
		assert!(!got.is_empty(), "expected dimmed dash, got empty");
	}

	#[test]
	fn timestamp_parses_rfc3339() {
		let got = timestamp("2024-01-01T12:00:00Z");
		assert!(got.contains("2024-01-01"), "missing abs date: {got:?}");
		assert!(got.contains("ago"), "missing relative form: {got:?}");
	}

	#[test]
	fn uptime_exact_zero_returns_dim_dash() {
		let got = uptime_exact(0);
		assert!(!got.is_empty(), "expected dimmed dash, got empty");
	}

	#[test]
	fn uptime_exact_negative_returns_dim_dash() {
		let got = uptime_exact(-1);
		assert!(!got.is_empty(), "expected dimmed dash, got empty");
	}

	#[test]
	fn uptime_exact_positive_includes_human_and_raw() {
		let got = uptime_exact(61_000);
		assert!(got.contains("1m 1s"), "missing human form: {got:?}");
		assert!(got.contains("61000 ms"), "missing raw ms: {got:?}");
	}

	// --- supplementary cases ------------------------------------------------

	#[test]
	fn bytes_large_units_terabytes_and_petabytes() {
		// Lock the bucket boundaries for the higher units: the parser must
		// keep dividing until the divisor fits, not stop at the first tier.
		assert_eq!(bytes(1024_i64.pow(4)), "1.0 TB");
		assert_eq!(bytes(1024_i64.pow(5)), "1.0 PB");
	}

	#[test]
	fn uptime_days_and_hours_combined() {
		// 2 days + 3 hours → "2d 3h"
		let got = uptime(2 * 86_400_000 + 3 * 3_600_000);
		assert_eq!(got, "2d 3h");
	}

	#[test]
	fn uptime_negative_returns_dim_dash_via_term() {
		let got = uptime(-5);
		assert!(!got.is_empty(), "expected dimmed dash, got empty");
	}

	#[test]
	fn timestamp_invalid_falls_back_to_raw_string() {
		let got = timestamp("not a timestamp");
		assert_eq!(got, "not a timestamp");
	}
}
