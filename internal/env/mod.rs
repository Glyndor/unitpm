//! `.env` parsing and process-environment helpers.
//!
//! `parse_file` reads a dotenv file into a `HashMap`, handling comments,
//! single- and double-quoted values, escape sequences, and the
//! space-or-tab-before-`#` rule that distinguishes an inline comment from a
//! literal `#` in the value. The quoting rules are deliberately permissive —
//! the Go test file enumerates the edge cases we have to honour.

use std::collections::HashMap;
use std::fs;
use std::path::Path;

/// Parse a `.env` file into a key/value map.
///
/// Blank lines and full-line comments (lines starting with `#`) are skipped.
/// Each non-blank line is split on the first `=`; the key is trimmed, the
/// value is parsed through [`parse_value`] which honours single quotes,
/// double quotes, and inline `# comments`.
pub fn parse_file(path: impl AsRef<Path>) -> Result<HashMap<String, String>, std::io::Error> {
	let text = fs::read_to_string(path)?;
	Ok(parse_str(&text))
}

/// In-memory parse, exposed so tests and direct callers can avoid the file.
#[must_use]
pub fn parse_str(text: &str) -> HashMap<String, String> {
	let mut out = HashMap::new();
	for raw_line in text.lines() {
		let line = raw_line.trim();
		if line.is_empty() || line.starts_with('#') {
			continue;
		}
		if let Some((k, v)) = parse_line(line) {
			out.insert(k, v);
		}
	}
	out
}

fn parse_line(line: &str) -> Option<(String, String)> {
	let (key_raw, value_raw) = line.split_once('=')?;
	let key = key_raw.trim();
	if key.is_empty() {
		return None;
	}
	let value = parse_value(value_raw.trim());
	Some((key.to_string(), value))
}

fn parse_value(value: &str) -> String {
	if value.is_empty() {
		return String::new();
	}
	let first = value.chars().next().expect("non-empty");
	match first {
		'"' => parse_double_quoted(value),
		'\'' => parse_single_quoted(value),
		_ => parse_unquoted(value),
	}
}

fn parse_double_quoted(value: &str) -> String {
	let bytes = value.as_bytes();
	for i in 1..bytes.len() {
		if bytes[i] == b'"' && bytes[i - 1] != b'\\' {
			let content = &value[1..i];
			return content.replace("\\\"", "\"");
		}
	}
	value.to_string()
}

fn parse_single_quoted(value: &str) -> String {
	let bytes = value.as_bytes();
	for i in 1..bytes.len() {
		if bytes[i] == b'\'' {
			return value[1..i].to_string();
		}
	}
	value.to_string()
}

fn parse_unquoted(value: &str) -> String {
	for (i, ch) in value.char_indices() {
		if ch == '#' && i > 0 {
			let prev = value.as_bytes()[i - 1];
			if prev == b' ' || prev == b'\t' {
				return value[..i].trim().to_string();
			}
		}
	}
	value.trim().to_string()
}

/// Read `key` from the process environment as a positive integer, falling
/// back to `fallback` when unset, malformed, zero, or negative. Returned as
/// `i64` to match the Go `Int` signature on a 64-bit target.
#[must_use]
pub fn int(key: &str, fallback: i64) -> i64 {
	match std::env::var(key) {
		Ok(v) => v
			.parse::<i64>()
			.map_or(fallback, |n| if n > 0 { n } else { fallback }),
		Err(_) => fallback,
	}
}

/// 64-bit variant of [`int`], kept as a distinct name to mirror the Go
/// `Int64` callsite even though both return `i64`.
#[must_use]
pub fn int64(key: &str, fallback: i64) -> i64 {
	int(key, fallback)
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::io::Write;

	fn write_env_file(content: &str) -> tempfile::NamedTempFile {
		let mut f = tempfile::Builder::new()
			.prefix("unitpm-env-")
			.suffix(".env")
			.tempfile()
			.expect("tempfile");
		f.write_all(content.as_bytes()).expect("write");
		f
	}

	#[test]
	fn parse_file_basic_key_value() {
		let f = write_env_file("FOO=bar\nBAZ=qux\n");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("FOO").map(String::as_str), Some("bar"));
		assert_eq!(got.get("BAZ").map(String::as_str), Some("qux"));
	}

	#[test]
	fn parse_file_double_quoted() {
		let f = write_env_file(r#"KEY="hello world""#);
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some("hello world"));
	}

	#[test]
	fn parse_file_double_quoted_escaped_quote() {
		let f = write_env_file(r#"KEY="say \"hello\"""#);
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some(r#"say "hello""#));
	}

	#[test]
	fn parse_file_single_quoted() {
		let f = write_env_file("KEY='hello world'");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some("hello world"));
	}

	#[test]
	fn parse_file_unquoted_with_inline_comment() {
		let f = write_env_file("KEY=value # this is a comment");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some("value"));
	}

	#[test]
	fn parse_file_unquoted_hash_in_value() {
		// Hash without preceding space is part of the value.
		let f = write_env_file("KEY=val#ue");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some("val#ue"));
	}

	#[test]
	fn parse_file_empty_value() {
		let f = write_env_file("KEY=");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some(""));
	}

	#[test]
	fn parse_file_skips_full_line_comments() {
		let f = write_env_file("# comment\nKEY=val\n# another comment\n");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.len(), 1);
		assert_eq!(got.get("KEY").map(String::as_str), Some("val"));
	}

	#[test]
	fn parse_file_skips_blank_lines() {
		let f = write_env_file("\n\nKEY=val\n\n");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.len(), 1);
	}

	#[test]
	fn parse_file_missing_file_errors() {
		let dir = tempfile::tempdir().expect("tempdir");
		let missing = dir.path().join("nonexistent.env");
		let err = parse_file(&missing);
		assert!(err.is_err(), "expected error for missing file");
	}

	#[test]
	fn parse_file_skips_lines_without_equals() {
		let f = write_env_file("NOEQUALS\nKEY=val\n");
		let got = parse_file(f.path()).expect("parse");
		assert!(!got.contains_key("NOEQUALS"));
		assert_eq!(got.get("KEY").map(String::as_str), Some("val"));
	}

	#[test]
	fn parse_file_only_splits_on_first_equals() {
		let f = write_env_file("KEY=a=b=c");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some("a=b=c"));
	}

	#[test]
	fn parse_file_trims_whitespace_around_key() {
		let f = write_env_file("  KEY  =value\n");
		let got = parse_file(f.path()).expect("parse");
		assert_eq!(got.get("KEY").map(String::as_str), Some("value"));
	}

	#[test]
	fn int_parses_positive_or_returns_fallback() {
		// Use a unique key per branch so tests don't collide on a real env.
		std::env::set_var("UNITPM_TEST_INT_OK", "42");
		assert_eq!(int("UNITPM_TEST_INT_OK", 99), 42);
		assert_eq!(int("UNITPM_TEST_INT_MISSING", 99), 99);
		std::env::set_var("UNITPM_TEST_INT_BAD", "nope");
		assert_eq!(int("UNITPM_TEST_INT_BAD", 99), 99);
		std::env::set_var("UNITPM_TEST_INT_ZERO", "0");
		assert_eq!(int("UNITPM_TEST_INT_ZERO", 99), 99);
		std::env::set_var("UNITPM_TEST_INT_NEG", "-5");
		assert_eq!(int("UNITPM_TEST_INT_NEG", 99), 99);
		std::env::remove_var("UNITPM_TEST_INT_OK");
		std::env::remove_var("UNITPM_TEST_INT_BAD");
		std::env::remove_var("UNITPM_TEST_INT_ZERO");
		std::env::remove_var("UNITPM_TEST_INT_NEG");
	}

	#[test]
	fn int64_parses_positive_or_returns_fallback() {
		std::env::set_var("UNITPM_TEST_I64", "1073741824");
		assert_eq!(int64("UNITPM_TEST_I64", 0), 1_073_741_824);
		assert_eq!(int64("UNITPM_TEST_I64_MISS", 123), 123);
		std::env::remove_var("UNITPM_TEST_I64");
	}
}
