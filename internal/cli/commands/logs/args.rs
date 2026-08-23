//! Argument parsing, target resolution, and filter construction for `unitpm logs`.
//!
//! Parsing mirrors the Go hand-rolled CLI: flag position is unrestricted
//! (flags may appear before, after, or interleaved with the positional
//! target), `--grep` / `--since` consume the next token, the first
//! non-flag argument is the target, and the missing-target case is an
//! error. Resolution finds the on-disk spec by namespace, ID, name, or
//! unique-ID-prefix and translates the parsed options into the
//! runtime's [`Filter`] and `[StreamSource]` set.

use std::time::Duration;

use crate::cli::errs::UsageError;
use crate::ipc::protocol::AppSpec;
use crate::spec;
use crate::types::DEFAULT_NAMESPACE;

use super::entry;
use super::merge::{Filter, StreamSource};

use super::DEFAULT_LINES;

/// Parse a human duration like `"1h30m"` into a [`std::time::Duration`].
/// Mirrors Go's `time.ParseDuration`, which the original code delegates
/// to via `time.ParseDuration`. The supported units are ns/us/ms/s/m/h,
/// optionally with a fractional value.
pub(super) fn parse_human_duration(s: &str) -> Result<Duration, String> {
	let bytes = s.as_bytes();
	let mut i = 0;
	let mut total_nanos: u128 = 0;
	while i < bytes.len() {
		let mut j = i;
		while j < bytes.len() && (bytes[j].is_ascii_digit() || bytes[j] == b'.') {
			j += 1;
		}
		if j == i {
			return Err(format!("missing number at position {i}"));
		}
		let num_str = &s[i..j];
		let num: f64 = num_str
			.parse()
			.map_err(|e| format!("invalid number {num_str:?}: {e}"))?;
		if j >= bytes.len() {
			return Err("missing unit suffix".into());
		}
		let unit = bytes[j];
		let nanos = match unit {
			b'n' => (num * 1.0) as u128,
			b'u' => (num * 1_000.0) as u128,
			b'm' => {
				if j + 1 < bytes.len() && bytes[j + 1] == b's' {
					// "ms" — milliseconds.
					let v = (num * 1_000_000.0) as u128;
					i = j + 2;
					total_nanos = total_nanos.checked_add(v).ok_or("overflow")?;
					continue;
				}
				(num * 60_000_000_000.0) as u128
			}
			b's' => (num * 1_000_000_000.0) as u128,
			b'h' => (num * 3_600_000_000_000.0) as u128,
			_ => return Err(format!("unknown unit {unit:?}")),
		};
		total_nanos = total_nanos.checked_add(nanos).ok_or("overflow")?;
		i = j + 1;
	}
	if total_nanos == 0 {
		return Err("empty duration".into());
	}
	Ok(Duration::from_nanos(total_nanos as u64))
}

/// Parsed flags for the `logs` command.
#[derive(Debug, Clone)]
pub struct Options {
	pub lines: usize,
	pub follow: bool,
	pub all: bool,
	pub yes: bool,
	pub no_merge: bool,
	pub since: Option<Duration>,
	pub grep: Option<String>,
	pub target: String,
	pub show_stdout: bool,
	pub show_stderr: bool,
	pub explicit: bool,
}

impl Default for Options {
	fn default() -> Self {
		Self {
			lines: DEFAULT_LINES,
			follow: false,
			all: false,
			yes: false,
			no_merge: false,
			since: None,
			grep: None,
			target: String::new(),
			show_stdout: false,
			show_stderr: false,
			explicit: false,
		}
	}
}

/// Parse the command's positional + flag arguments. Mirrors the Go
/// hand-rolled parser: flag position is unrestricted, `--grep` and
/// `--since` consume the next token, and the first non-flag argument is
/// the target.
pub fn parse_args(args: &[String]) -> Result<Options, Box<dyn std::error::Error + Send + Sync>> {
	let mut opts = Options::default();
	let mut i = 0;
	while i < args.len() {
		let arg = &args[i];
		match arg.as_str() {
			"--lines" | "-n" | "--tail" => {
				if i + 1 < args.len() {
					if let Ok(v) = args[i + 1].parse::<usize>() {
						opts.lines = v;
						i += 1;
					}
				}
			}
			"--follow" | "-f" => opts.follow = true,
			"--all" => opts.all = true,
			"--yes" | "-y" => opts.yes = true,
			"--no-merge" => opts.no_merge = true,
			"--since" => {
				if i + 1 < args.len() {
					match parse_human_duration(&args[i + 1]) {
						Ok(v) => {
							opts.since = Some(v);
							i += 1;
						}
						Err(e) => {
							return Err(Box::<dyn std::error::Error + Send + Sync>::from(
								UsageError::new(format!(
									"invalid --since duration {:?}: {e}",
									args[i + 1]
								)),
							));
						}
					}
				}
			}
			"--grep" | "-g" => {
				if i + 1 < args.len() {
					opts.grep = Some(args[i + 1].clone());
					i += 1;
				}
			}
			"--stdout" | "-o" => {
				opts.show_stdout = true;
				opts.explicit = true;
			}
			"--stderr" | "-e" => {
				opts.show_stderr = true;
				opts.explicit = true;
			}
			_ if !arg.starts_with('-') => {
				opts.target = arg.clone();
			}
			_ => {}
		}
		i += 1;
	}
	if !opts.explicit {
		opts.show_stdout = true;
		opts.show_stderr = true;
	}
	if opts.target.is_empty() {
		return Err(Box::<dyn std::error::Error + Send + Sync>::from(
			UsageError::new("missing process ID or name"),
		));
	}
	Ok(opts)
}

/// Locate the on-disk spec matching the user's target. Supports
/// `namespace:name`, exact ID, exact name, and unique-ID-prefix matches.
/// Returns the first match found; an ambiguous prefix returns an error.
pub fn resolve_target(target: &str) -> Result<AppSpec, Box<dyn std::error::Error + Send + Sync>> {
	let (namespace, name_or_id) = match target.find(':') {
		Some(idx) => (target[..idx].to_string(), target[idx + 1..].to_string()),
		None => (String::new(), target.to_string()),
	};

	let specs = spec::load_all_protocol().map_err(|e| {
		Box::<dyn std::error::Error + Send + Sync>::from(format!("failed to load specs: {e}"))
	})?;

	let mut matched: Option<AppSpec> = None;
	for s in &specs {
		let ns = s.namespace.as_deref().unwrap_or("");
		let ns = if ns.is_empty() { DEFAULT_NAMESPACE } else { ns };
		if !namespace.is_empty() && ns != namespace {
			continue;
		}
		if s.id == name_or_id || s.name == name_or_id || s.id.starts_with(&name_or_id) {
			if let Some(prev) = &matched {
				if prev.id != s.id {
					return Err(Box::<dyn std::error::Error + Send + Sync>::from(format!(
						"ambiguous argument {target:?}: matches multiple processes"
					)));
				}
			}
			matched = Some(s.clone());
		}
	}
	matched.ok_or_else(|| {
		Box::<dyn std::error::Error + Send + Sync>::from(format!("process {target:?} not found"))
	})
}

/// Build the list of stream sources from the resolved spec.
pub fn build_sources(spec: &AppSpec, opts: &Options) -> Vec<StreamSource> {
	let mut logs_dir = String::new();
	let mut stdout = String::new();
	let mut stderr = String::new();
	if let Some(l) = &spec.logs {
		logs_dir = l.dir.clone().unwrap_or_default();
		stdout = l.stdout.clone().unwrap_or_default();
		stderr = l.stderr.clone().unwrap_or_default();
	}
	let (stdout_path, stderr_path) =
		crate::paths::resolve_log_paths(&spec.id, &logs_dir, &stdout, &stderr)
			.map(|(s, e)| {
				(
					s.to_string_lossy().into_owned(),
					e.to_string_lossy().into_owned(),
				)
			})
			.unwrap_or_else(|_| (String::new(), String::new()));

	let mut out = Vec::new();
	if opts.show_stdout {
		out.push(StreamSource::new(stdout_path.clone(), "STDOUT"));
	}
	if opts.show_stderr && stderr_path != stdout_path {
		out.push(StreamSource::new(stderr_path, "STDERR"));
	}
	out
}

/// Translate the parsed filter options into the [`Filter`] struct.
pub fn build_filter(opts: &Options) -> Result<Filter, Box<dyn std::error::Error + Send + Sync>> {
	let mut fs = Filter::default();
	if let Some(d) = opts.since {
		fs.since = Some(entry::now_unix() - d.as_secs() as i64);
	}
	if let Some(pat) = &opts.grep {
		let re = regex::Regex::new(pat).map_err(|e| {
			Box::<dyn std::error::Error + Send + Sync>::from(format!("invalid --grep regex: {e}"))
		})?;
		fs.grep = Some(re);
	}
	Ok(fs)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn parse_human_duration_units() {
		assert_eq!(parse_human_duration("1s").unwrap(), Duration::from_secs(1));
		assert_eq!(
			parse_human_duration("30ms").unwrap(),
			Duration::from_millis(30)
		);
		assert_eq!(
			parse_human_duration("2m").unwrap(),
			Duration::from_secs(120)
		);
		assert_eq!(
			parse_human_duration("1h").unwrap(),
			Duration::from_secs(3600)
		);
		// Composite — sums each component.
		let got = parse_human_duration("1h30m").unwrap();
		assert_eq!(got, Duration::from_secs(3600 + 30 * 60));
	}

	#[test]
	fn parse_args_defaults() {
		let opts = parse_args(&["api".into()]).unwrap();
		assert_eq!(opts.lines, 40);
		assert!(!opts.follow && !opts.all);
		assert!(opts.show_stdout && opts.show_stderr);
		assert!(!opts.explicit);
		assert_eq!(opts.target, "api");
	}

	#[test]
	fn parse_args_tail_flag() {
		let opts = parse_args(&["api".into(), "--tail".into(), "100".into()]).unwrap();
		assert_eq!(opts.lines, 100);
	}

	#[test]
	fn parse_args_short_n() {
		let opts = parse_args(&["api".into(), "-n".into(), "7".into()]).unwrap();
		assert_eq!(opts.lines, 7);
	}

	#[test]
	fn parse_args_since_duration() {
		let opts = parse_args(&["api".into(), "--since".into(), "1h".into()]).unwrap();
		assert_eq!(opts.since, Some(Duration::from_secs(3600)));
	}

	#[test]
	fn parse_args_invalid_since() {
		let err = parse_args(&["api".into(), "--since".into(), "invalid".into()]).unwrap_err();
		let msg = err.to_string();
		assert!(msg.contains("invalid --since"), "{msg}");
	}

	#[test]
	fn parse_args_missing_target() {
		let err = parse_args(&["-f".into()]).unwrap_err();
		assert!(err.to_string().contains("missing process"), "got: {err}");
	}

	#[test]
	fn parse_args_all_flags() {
		let opts = parse_args(&[
			"api".into(),
			"--all".into(),
			"--yes".into(),
			"--grep".into(),
			"ERROR".into(),
			"--stderr".into(),
			"--no-merge".into(),
			"-n".into(),
			"200".into(),
		])
		.unwrap();
		assert!(opts.all);
		assert!(opts.yes);
		assert_eq!(opts.grep.as_deref(), Some("ERROR"));
		assert!(opts.no_merge);
		assert_eq!(opts.lines, 200);
		assert!(opts.show_stderr);
		assert!(!opts.show_stdout);
	}

	#[test]
	fn parse_args_short_grep() {
		let opts = parse_args(&["api".into(), "-g".into(), "panic".into()]).unwrap();
		assert_eq!(opts.grep.as_deref(), Some("panic"));
	}

	#[test]
	fn build_filter_bad_regex_errors() {
		let opts = Options {
			grep: Some("([".to_string()),
			..Default::default()
		};
		assert!(build_filter(&opts).is_err());
	}

	#[test]
	fn build_filter_since_clock() {
		let opts = Options {
			since: Some(Duration::from_secs(3600)),
			..Default::default()
		};
		let fs = build_filter(&opts).unwrap();
		assert!(fs.since.is_some());
		// Should be approximately now - 1h.
		let cutoff = fs.since.unwrap();
		let drift = entry::now_unix() - cutoff - 3600;
		assert!(drift.abs() < 5);
	}
}
