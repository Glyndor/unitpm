//! Time helpers for the `start` command.
//!
//! The original Go code uses `time.Now().Format(time.RFC3339)` for the
//! created-at timestamp. We construct the wire format inline rather
//! than drag chrono through the dependency chain. The suite already
//! has rfc3339 helpers that round-trip in [`crate::cli::format`], but
//! for our purposes we just need a single moment formatted as
//! RFC 3339 in UTC.

/// Format the current instant as an RFC 3339 timestamp with `Z`
/// (UTC) suffix and second precision. Returns
/// `"1970-01-01T00:00:00Z"` if the system clock predates the Unix
/// epoch, which would otherwise panic through
/// `duration_since(UNIX_EPOCH)`.
pub(crate) fn now_rfc3339() -> String {
	use std::time::{SystemTime, UNIX_EPOCH};
	let secs = SystemTime::now()
		.duration_since(UNIX_EPOCH)
		.map(|d| d.as_secs() as i64)
		.unwrap_or(0);
	let (year, month, day, hour, min, sec) = unix_to_ymdhms(secs);
	format!(
		"{:04}-{:02}-{:02}T{:02}:{:02}:{:02}Z",
		year, month, day, hour, min, sec
	)
}

/// Convert Unix seconds to (year, month, day, hour, min, sec) in UTC.
/// Tiny implementation to keep the start command free of chrono
/// dependencies. Seconds since epoch, Gregorian calendar, UTC.
///
/// Algorithm from Howard Hinnant's `civil_from_days`, inlined.
pub(crate) fn unix_to_ymdhms(secs: i64) -> (i32, u32, u32, u32, u32, u32) {
	let days = secs.div_euclid(86_400);
	let mut secs_of_day = secs.rem_euclid(86_400) as u32;
	let hour = secs_of_day / 3600;
	secs_of_day %= 3600;
	let minute = secs_of_day / 60;
	let second = secs_of_day % 60;

	let z = days + 719_468;
	let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
	let doe = (z - era * 146_097) as u64;
	let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
	let y = yoe as i64 + era * 400;
	let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	let mp = (5 * doy + 2) / 153;
	let d = doy - (153 * mp + 2) / 5 + 1;
	let m = if mp < 10 { mp + 3 } else { mp - 9 };
	let year = (if m <= 2 { y + 1 } else { y }) as i32;

	(year, m as u32, d as u32, hour, minute, second)
}
