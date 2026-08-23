//! Memory-size parser for the `start` command's `--memory-max` flag.
//!
//! Accepts values like "512M", "2G", "1024k", "10485760". K, M, G are
//! base 1024, case-insensitive. Empty and whitespace-only strings
//! return 0; the spec-construction code treats that as "no limit set".

/// Parse a memory-size expression. Empty / whitespace-only inputs
/// return 0. K/M/G are case-insensitive base-1024 multipliers.
pub fn parse_memory_size(s: &str) -> Result<i64, String> {
	let trimmed = s.trim();
	if trimmed.is_empty() {
		return Ok(0);
	}
	let bytes = trimmed.as_bytes();
	let last = bytes[bytes.len() - 1] as char;
	let (mult, value_str) = match last {
		'k' | 'K' => (1024i64, &trimmed[..trimmed.len() - 1]),
		'm' | 'M' => (1024 * 1024, &trimmed[..trimmed.len() - 1]),
		'g' | 'G' => (1024 * 1024 * 1024, &trimmed[..trimmed.len() - 1]),
		_ => (1, trimmed),
	};
	let value_str = value_str.trim();
	let n: i64 = value_str
		.parse()
		.map_err(|_| format!("invalid memory size {value_str:?} (expected e.g. 512M or 2G)"))?;
	if n <= 0 {
		return Err(format!(
			"invalid memory size {value_str:?} (expected e.g. 512M or 2G)"
		));
	}
	Ok(n * mult)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn empty_returns_zero() {
		assert_eq!(parse_memory_size("").unwrap(), 0);
	}

	#[test]
	fn whitespace_returns_zero() {
		assert_eq!(parse_memory_size("   ").unwrap(), 0);
	}

	#[test]
	fn kilobytes() {
		assert_eq!(parse_memory_size("512k").unwrap(), 512 * 1024);
		assert_eq!(parse_memory_size("512K").unwrap(), 512 * 1024);
		assert_eq!(parse_memory_size("1K").unwrap(), 1024);
	}

	#[test]
	fn megabytes() {
		assert_eq!(parse_memory_size("512m").unwrap(), 512 * 1024 * 1024);
		assert_eq!(parse_memory_size("512M").unwrap(), 512 * 1024 * 1024);
		assert_eq!(parse_memory_size("1M").unwrap(), 1024 * 1024);
	}

	#[test]
	fn gigabytes() {
		assert_eq!(parse_memory_size("2G").unwrap(), 2i64 * 1024 * 1024 * 1024);
	}

	#[test]
	fn raw_bytes() {
		assert_eq!(parse_memory_size("10485760").unwrap(), 10485760);
	}

	#[test]
	fn invalid_inputs_error() {
		for s in ["abc", "0M", "-1M", "0"] {
			assert!(parse_memory_size(s).is_err(), "expected error for {s}");
		}
	}
}
