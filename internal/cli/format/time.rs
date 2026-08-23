//! RFC3339 timestamp parsing and local-time formatting.
//!
//! Pulled out of [`super`] so the public surface stays focused on
//! formatters. The Go side uses `time.Parse(time.RFC3339, ts)`; we hand-roll
//! the parser because pulling in `chrono` / `time` would dwarf the
//! formatter module by an order of magnitude and the only shape the CLI
//! actually receives is the daemon-emitted `"2024-01-01T12:00:00Z"` and
//! its RFC3339-nano sibling.

use std::time::{SystemTime, UNIX_EPOCH};

/// Convert an RFC3339 (or RFC3339-nano) string into Unix seconds (UTC).
/// `None` on any parse failure; the caller is expected to fall back to
/// the raw string.
#[must_use]
pub fn parse_rfc3339(ts: &str) -> Option<i64> {
	let (date, rest) = ts.split_once('T')?;
	let date_parts: Vec<&str> = date.split('-').collect();
	if date_parts.len() != 3 {
		return None;
	}
	let year: i64 = date_parts[0].parse().ok()?;
	let month: i64 = date_parts[1].parse().ok()?;
	let day: i64 = date_parts[2].parse().ok()?;

	// Split off the timezone suffix; the rest is the time portion. Find
	// the first `+`, `-` (after position 2 so the date `-`s are skipped),
	// or `Z`.
	let (time_str, tz_offset_secs) = split_tz(rest)?;

	let time_parts: Vec<&str> = time_str.split(':').collect();
	if time_parts.len() != 3 {
		return None;
	}
	let hour: i64 = time_parts[0].parse().ok()?;
	let minute: i64 = time_parts[1].parse().ok()?;
	let sec_str = time_parts[2];
	let (sec, frac) = match sec_str.split_once('.') {
		Some((s, f)) => {
			let sec: i64 = s.parse().ok()?;
			// Convert fractional seconds to nanoseconds.
			let mut f = f.to_string();
			while f.len() < 9 {
				f.push('0');
			}
			let frac: i64 = f[..9].parse().unwrap_or(0);
			(sec, frac)
		}
		None => {
			let sec: i64 = sec_str.parse().ok()?;
			(sec, 0)
		}
	};

	let days = days_from_civil(year, month, day)?;
	let mut total_secs = days * 86_400 + hour * 3_600 + minute * 60 + sec;
	// Subtract the timezone offset to land in UTC. The parsed offset is
	// "local - UTC", so a +02:00 timestamp at 14:00 is 12:00 UTC.
	total_secs -= tz_offset_secs;
	// Round half-up the way RFC3339 implies when the fraction is past
	// half a second.
	if frac >= 500_000_000 {
		total_secs += 1;
	}
	Some(total_secs)
}

/// Now-as-Unix-seconds. Wraps the `SystemTime` dance so the caller stays
/// focused on the formatted string.
#[must_use]
pub fn now_unix_seconds() -> i64 {
	SystemTime::now()
		.duration_since(UNIX_EPOCH)
		.map(|d| d.as_secs() as i64)
		.unwrap_or(0)
}

/// Format a Unix timestamp as `"YYYY-MM-DD HH:MM:SS"` in the local timezone.
///
/// The Go original uses `t.Local()`; we honour the TZ environment variable
/// via the system libc so a `TZ=UTC cargo test` produces UTC output.
#[must_use]
pub fn format_unix_local(unix_secs: i64) -> String {
	// SAFETY: `localtime_r` writes into a `tm`; we only read the fields on
	// success and ignore any `errno`.
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
	let rc = unsafe {
		libc::localtime_r(
			&(unix_secs as libc::time_t),
			&mut tm as *mut Tm as *mut libc::tm,
		)
	};
	if rc.is_null() {
		return String::from("1970-01-01 00:00:00");
	}
	format!(
		"{:04}-{:02}-{:02} {:02}:{:02}:{:02}",
		tm.tm_year + 1900,
		tm.tm_mon + 1,
		tm.tm_mday,
		tm.tm_hour,
		tm.tm_min,
		tm.tm_sec
	)
}

/// Render a `Δ` from `now()` as a relative-age string (`"2h ago"`,
/// `"just now"`, ...).
#[must_use]
pub fn relative_age(delta_secs: i64) -> String {
	match () {
		_ if delta_secs < 0 => "in the future".to_string(),
		_ if delta_secs < 60 => "just now".to_string(),
		_ if delta_secs < 3_600 => format!("{}m ago", delta_secs / 60),
		_ if delta_secs < 86_400 => format!("{}h ago", delta_secs / 3_600),
		_ => format!("{}d ago", delta_secs / 86_400),
	}
}

/// Splits the time-portion of an RFC3339 string at its timezone suffix,
/// returning the local time string and the offset in seconds east of UTC.
///
/// The timezone suffix is required — RFC3339 calls for one of `Z`,
/// `+HH:MM`, or `-HH:MM`. A bare time like `"2024-01-01T12:00:00"` is
/// rejected, matching `time.Parse(time.RFC3339, ...)`'s behaviour on the
/// Go side.
fn split_tz(rest: &str) -> Option<(&str, i64)> {
	let bytes = rest.as_bytes();
	let mut tz_start = None;
	for (i, &b) in bytes.iter().enumerate() {
		if i < 2 {
			continue;
		}
		if b == b'Z' || b == b'+' || b == b'-' {
			tz_start = Some(i);
			break;
		}
	}
	let i = tz_start?;
	let offset = parse_tz(&rest[i..])?;
	Some((&rest[..i], offset))
}

fn parse_tz(tz: &str) -> Option<i64> {
	if tz.is_empty() || tz == "Z" {
		return Some(0);
	}
	let sign = match tz.as_bytes()[0] {
		b'+' => 1,
		b'-' => -1,
		_ => return None,
	};
	let body = &tz[1..];
	let (hh, mm) = body.split_once(':')?;
	let h: i64 = hh.parse().ok()?;
	let m: i64 = mm.parse().ok()?;
	Some(sign * (h * 3_600 + m * 60))
}

/// Howard Hinnant's days_from_civil — number of days since the Unix epoch
/// for the given proleptic-Gregorian date. Negative for dates before 1970.
fn days_from_civil(y: i64, m: i64, d: i64) -> Option<i64> {
	let y = if m <= 2 { y - 1 } else { y };
	let era = if y >= 0 { y } else { y - 399 } / 400;
	let yoe = (y - era * 400) as u64; // [0, 399]
	let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + d - 1;
	let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy as u64; // [0, 146096]
	let days = era * 146_097 + doe as i64 - 719_468;
	Some(days)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn parse_rfc3339_basic_zulu() {
		let got = parse_rfc3339("2024-01-01T12:00:00Z").expect("parse");
		// 2024-01-01 12:00:00 UTC = 1704110400.
		assert_eq!(got, 1_704_110_400);
	}

	#[test]
	fn parse_rfc3339_with_positive_offset() {
		// 14:00 +02:00 == 12:00 UTC
		let got = parse_rfc3339("2024-01-01T14:00:00+02:00").expect("parse");
		assert_eq!(got, 1_704_110_400);
	}

	#[test]
	fn parse_rfc3339_with_negative_offset() {
		// 07:00 -05:00 == 12:00 UTC
		let got = parse_rfc3339("2024-01-01T07:00:00-05:00").expect("parse");
		assert_eq!(got, 1_704_110_400);
	}

	#[test]
	fn parse_rfc3339_with_nano_fraction() {
		// Half-up rounding: .5s and above → +1s.
		let got = parse_rfc3339("2024-01-01T12:00:00.5Z").expect("parse");
		assert_eq!(got, 1_704_110_401);
		let got = parse_rfc3339("2024-01-01T12:00:00.499999999Z").expect("parse");
		assert_eq!(got, 1_704_110_400);
	}

	#[test]
	fn parse_rfc3339_invalid_returns_none() {
		assert!(parse_rfc3339("not a timestamp").is_none());
		assert!(parse_rfc3339("2024-01-01").is_none());
		assert!(parse_rfc3339("2024-01-01T12:00:00").is_none());
		assert!(parse_rfc3339("2024-01-01T12:00:00+bad").is_none());
	}

	#[test]
	fn relative_age_buckets() {
		assert_eq!(relative_age(-1), "in the future");
		assert_eq!(relative_age(0), "just now");
		assert_eq!(relative_age(30), "just now");
		assert_eq!(relative_age(120), "2m ago");
		assert_eq!(relative_age(3_600), "1h ago");
		assert_eq!(relative_age(86_400), "1d ago");
	}
}
