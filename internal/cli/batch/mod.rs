//! Shared result shape for multi-target commands (stop, restart, ...).
//!
//! 13 cases ported from `internal/cli/batch/batch_test.go`.
//!
//! The module owns three concerns:
//!
//!   - JSON output (`{ "op": ..., "results": [...], "summary": {...} }`)
//!   - Non-zero exit when any target failed
//!   - Optional human-readable trailing summary when more than one target
//!     is involved
//!
//! Each per-target human line is emitted by the calling command as results
//! arrive; batch only owns the aggregate shape and the final reporting.

use std::collections::BTreeMap;
use std::io::{self, Write};

use serde::Serialize;

use crate::jsonx;
use crate::term;

/// Partition `args` into flag-like tokens (anything starting with `-`) and
/// positional tokens. Lets users put `--json` / `--purge` either before or
/// after the target IDs.
///
/// Safe ONLY for commands whose flags are all boolean (no `--key value`
/// pairs). Use [`split_args_with_values`] when the command accepts
/// value-taking flags like `--namespace prod`.
#[must_use]
pub fn split_args(args: &[String]) -> (Vec<String>, Vec<String>) {
	split_args_with_values(args, &[])
}

/// Like [`split_args`] but aware of value-taking flag names. Pass the long
/// flag names (without leading dashes) that consume the next token as
/// their value, e.g. `&["namespace".to_string()].iter().map(|s| ...).collect()`.
/// The function recognises both `--namespace prod` (two tokens) and
/// `--namespace=prod` (one token). Unknown long flags fall back to the
/// boolean classification used by [`split_args`].
#[must_use]
pub fn split_args_with_values(
	args: &[String],
	value_flags: &[String],
) -> (Vec<String>, Vec<String>) {
	let mut flags: Vec<String> = Vec::new();
	let mut positional: Vec<String> = Vec::new();
	let mut i = 0;
	while i < args.len() {
		let a = &args[i];
		if a.len() > 1 && a.starts_with('-') {
			flags.push(a.clone());
			if a.contains('=') {
				i += 1;
				continue;
			}
			let name = a.trim_start_matches('-');
			if value_flags.iter().any(|n| n == name) && i + 1 < args.len() {
				flags.push(args[i + 1].clone());
				i += 2;
				continue;
			}
		} else {
			positional.push(a.clone());
		}
		i += 1;
	}
	(flags, positional)
}

/// Status of a single target's outcome.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Status {
	/// Operation completed with effect.
	Ok,
	/// Daemon returned an error for this target.
	Failed,
	/// Operation was a no-op (e.g. already stopped).
	Noop,
}

impl Status {
	#[must_use]
	pub const fn as_str(self) -> &'static str {
		match self {
			Status::Ok => "ok",
			Status::Failed => "failed",
			Status::Noop => "noop",
		}
	}
}

/// One target's outcome.
#[derive(Debug, Clone, PartialEq, Serialize)]
pub struct Result {
	pub id: String,
	pub status: Status,
	#[serde(skip_serializing_if = "Option::is_none")]
	pub error: Option<String>,
	#[serde(skip_serializing_if = "BTreeMap::is_empty")]
	pub extra: BTreeMap<String, serde_json::Value>,
}

/// Counts per status. The Go wire format omits `noop` when zero; the
/// `serde` annotation preserves that shape.
#[derive(Debug, Clone, Default, PartialEq, Serialize)]
pub struct Summary {
	pub total: usize,
	pub ok: usize,
	pub failed: usize,
	#[serde(skip_serializing_if = "is_zero")]
	pub noop: usize,
}

fn is_zero(n: &usize) -> bool {
	*n == 0
}

/// Aggregate for one batch run.
#[derive(Debug, Clone, PartialEq, Serialize)]
pub struct Report {
	pub op: String,
	pub results: Vec<Result>,
	pub summary: Summary,
}

impl Report {
	/// Start a fresh report for `op`.
	#[must_use]
	pub fn new(op: &str) -> Self {
		Self {
			op: op.to_string(),
			results: Vec::new(),
			summary: Summary::default(),
		}
	}

	/// Append `res` and update counters.
	pub fn add(&mut self, res: Result) {
		self.summary.total += 1;
		match res.status {
			Status::Ok => self.summary.ok += 1,
			Status::Failed => self.summary.failed += 1,
			Status::Noop => self.summary.noop += 1,
		}
		self.results.push(res);
	}

	/// Record a successful target. `extra` may carry command-specific
	/// payload (e.g. bytes freed, was_running) that the caller wants
	/// surfaced in `--json` output.
	pub fn ok(&mut self, id: &str, extra: BTreeMap<String, serde_json::Value>) {
		self.add(Result {
			id: id.to_string(),
			status: Status::Ok,
			error: None,
			extra,
		});
	}

	/// Record a target that was already in the desired state.
	pub fn noop(&mut self, id: &str, extra: BTreeMap<String, serde_json::Value>) {
		self.add(Result {
			id: id.to_string(),
			status: Status::Noop,
			error: None,
			extra,
		});
	}

	/// Record a target that errored.
	pub fn fail(&mut self, id: &str, err: Option<&dyn std::fmt::Display>) {
		let msg = err.map(|e| e.to_string());
		self.add(Result {
			id: id.to_string(),
			status: Status::Failed,
			error: msg,
			extra: BTreeMap::new(),
		});
	}

	/// Error suitable for the command's return value: `None` when nothing
	/// failed, `"<op> failed"` for a single-target invocation, and
	/// `"<op>: N of M targets failed"` for the batch shape.
	#[must_use]
	pub fn err(&self) -> Option<BatchError> {
		if self.summary.failed == 0 {
			return None;
		}
		if self.summary.failed == 1 && self.summary.total == 1 {
			return Some(BatchError(format!("{} failed", self.op)));
		}
		Some(BatchError(format!(
			"{}: {} of {} targets failed",
			self.op, self.summary.failed, self.summary.total
		)))
	}

	/// Emit the trailing one-line summary when more than one target was
	/// processed. Single-target invocations stay silent so the common path
	/// remains terse.
	pub fn print_summary<W: Write>(&self, w: &mut W) -> io::Result<()> {
		if self.summary.total <= 1 {
			return Ok(());
		}
		let mut parts: Vec<String> = Vec::new();
		parts.push(format!("{} ok", self.summary.ok));
		if self.summary.noop > 0 {
			parts.push(term::yellow(format_args!("{} noop", self.summary.noop)));
		}
		if self.summary.failed > 0 {
			parts.push(term::red(format_args!("{} failed", self.summary.failed)));
		}
		let mark = if self.summary.failed == 0 {
			term::green(format_args!("{}", "✓"))
		} else {
			term::red(format_args!("{}", "✗"))
		};
		let op_bold = term::bold(format_args!("{}", self.op));
		writeln!(w, "\n{mark} {op_bold}: {}", parts.join(", "))
	}

	/// Convenience wrapper: lock stdout and call [`Self::print_summary`].
	pub fn print_summary_stdout(&self) -> io::Result<()> {
		let stdout = io::stdout();
		let mut out = stdout.lock();
		self.print_summary(&mut out)
	}

	/// Marshal the report as JSON and write it to `w` followed by a newline.
	pub fn emit_json_to<W: Write>(&self, w: &mut W) -> io::Result<()> {
		let bytes = jsonx::marshal(self).map_err(jsonx_error_to_io)?;
		w.write_all(&bytes)?;
		writeln!(w)
	}

	/// Convenience wrapper: lock stdout and call [`Self::emit_json_to`].
	pub fn emit_json(&self) -> io::Result<()> {
		let stdout = io::stdout();
		let mut out = stdout.lock();
		self.emit_json_to(&mut out)
	}
}

/// Error type returned by [`Report::err`]. String-payload — the
/// `Display` form is the operator-facing message.
#[derive(Debug)]
pub struct BatchError(pub String);

impl std::fmt::Display for BatchError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.write_str(&self.0)
	}
}

impl std::error::Error for BatchError {}

fn jsonx_error_to_io(e: jsonx::Error) -> io::Error {
	io::Error::new(io::ErrorKind::InvalidData, e)
}

#[cfg(test)]
mod tests {
	use std::sync::{Mutex, MutexGuard};

	use super::*;

	/// Process-global quiet flag. Tests that flip it must hold the lock
	/// and restore on `Drop` so a failing assertion cannot leave the next
	/// test running with the wrong state.
	static QUIET_LOCK: Mutex<()> = Mutex::new(());
	struct QuietGuard<'a> {
		_lock: MutexGuard<'a, ()>,
		prev: bool,
	}
	impl Drop for QuietGuard<'_> {
		fn drop(&mut self) {
			term::set_quiet(self.prev);
		}
	}

	fn lock_quiet() -> QuietGuard<'static> {
		// The Mutex is a 'static so we can return a guard tied to it; the
		// std Mutex does not poison on first call, but it can poison later
		// — recover gracefully rather than panic on Drop.
		let prev = term::is_quiet();
		term::set_quiet(false);
		QuietGuard {
			_lock: QUIET_LOCK.lock().unwrap_or_else(|e| e.into_inner()),
			prev,
		}
	}

	#[test]
	fn new_report_is_empty() {
		let r = Report::new("delete");
		assert_eq!(r.op, "delete");
		assert_eq!(r.summary.total, 0);
		assert_eq!(r.summary.ok, 0);
		assert_eq!(r.summary.failed, 0);
		assert_eq!(r.summary.noop, 0);
		assert!(r.err().is_none());
	}

	#[test]
	fn ok_records_and_counts() {
		let mut r = Report::new("reset");
		r.ok("a", BTreeMap::new());
		r.ok(
			"b",
			BTreeMap::from([("extra".into(), serde_json::json!("x"))]),
		);
		assert_eq!(r.summary.total, 2);
		assert_eq!(r.summary.ok, 2);
		assert!(r.err().is_none());
	}

	#[test]
	fn noop_counts_separately_and_does_not_error() {
		let mut r = Report::new("stop");
		r.ok("running-proc", BTreeMap::new());
		r.noop("already-stopped", BTreeMap::new());
		assert_eq!(r.summary.noop, 1);
		assert!(r.err().is_none());
	}

	#[test]
	fn fail_records_and_errors() {
		let mut r = Report::new("reload");
		r.fail("ghost", Some(&"not found"));
		let err = r.err().expect("must error when any target failed");
		assert!(
			err.to_string().contains("reload"),
			"error should mention op 'reload', got {err:?}"
		);
	}

	#[test]
	fn err_message_shapes_for_single_and_mixed() {
		// Single target failed → "<op> failed".
		let mut r1 = Report::new("stop");
		r1.fail("x", Some(&"boom"));
		assert_eq!(r1.err().expect("err").to_string(), "stop failed");

		// Mixed batch → "<op>: N of M targets failed".
		let mut r2 = Report::new("stop");
		r2.ok("a", BTreeMap::new());
		r2.fail("b", Some(&"boom"));
		r2.ok("c", BTreeMap::new());
		let msg = r2.err().expect("err").to_string();
		assert!(
			msg.contains("1 of 3"),
			"mixed err should report '1 of 3', got {msg:?}"
		);
	}

	#[test]
	fn emit_json_shape_matches_wire_format() {
		let mut r = Report::new("delete");
		r.ok(
			"abc",
			BTreeMap::from([("purged".into(), serde_json::json!(true))]),
		);
		r.fail("ghost", Some(&"not found"));

		let mut buf = Vec::new();
		r.emit_json_to(&mut buf).expect("emit");
		let got = String::from_utf8(buf).expect("utf8");

		let decoded: serde_json::Value = serde_json::from_str(&got).expect("json");
		assert_eq!(decoded["op"], "delete");
		assert_eq!(decoded["results"].as_array().map(Vec::len), Some(2));
		assert_eq!(decoded["results"][0]["status"], "ok");
		assert_eq!(decoded["results"][1]["status"], "failed");
		assert_eq!(decoded["results"][1]["error"], "not found");
		assert_eq!(decoded["results"][0]["extra"]["purged"], true);
		assert_eq!(decoded["summary"]["total"], 2);
		assert_eq!(decoded["summary"]["ok"], 1);
		assert_eq!(decoded["summary"]["failed"], 1);
	}

	#[test]
	fn print_summary_hidden_for_single_target() {
		let _g = lock_quiet();
		let mut r = Report::new("stop");
		r.ok("only-one", BTreeMap::new());
		let mut buf = Vec::new();
		r.print_summary(&mut buf).expect("summary");
		assert!(
			buf.is_empty(),
			"single-target should have no trailing summary, got {:?}",
			String::from_utf8_lossy(&buf)
		);
	}

	#[test]
	fn print_summary_visible_for_batch() {
		let _g = lock_quiet();
		let mut r = Report::new("stop");
		r.ok("a", BTreeMap::new());
		r.ok("b", BTreeMap::new());
		r.fail("c", Some(&"oops"));
		let mut buf = Vec::new();
		r.print_summary(&mut buf).expect("summary");
		let out = String::from_utf8_lossy(&buf);
		let plain = crate::cli::format::strip_ansi(&out);
		assert!(
			plain.contains("stop"),
			"summary should mention op, got {plain:?}"
		);
		assert!(
			plain.contains("2 ok"),
			"summary should count ok outcomes, got {plain:?}"
		);
		assert!(
			plain.contains("1 failed"),
			"summary should count failed outcomes, got {plain:?}"
		);
	}

	// --- arg splitter cases -------------------------------------------------

	#[test]
	fn split_args_boolean_separates_flags_and_positionals() {
		let args = vec![
			"a".to_string(),
			"--json".to_string(),
			"b".to_string(),
			"--purge".to_string(),
		];
		let (flags, pos) = split_args(&args);
		assert_eq!(flags.join(","), "--json,--purge");
		assert_eq!(pos.join(","), "a,b");
	}

	#[test]
	fn split_args_with_values_two_token_form() {
		let args = vec![
			"a".to_string(),
			"--namespace".to_string(),
			"prod".to_string(),
			"b".to_string(),
		];
		let value_flags = vec!["namespace".to_string()];
		let (flags, pos) = split_args_with_values(&args, &value_flags);
		assert_eq!(flags.join(","), "--namespace,prod");
		assert_eq!(pos.join(","), "a,b");
	}

	#[test]
	fn split_args_with_values_equals_form() {
		let args = vec!["--namespace=prod".to_string(), "api".to_string()];
		let value_flags = vec!["namespace".to_string()];
		let (flags, pos) = split_args_with_values(&args, &value_flags);
		assert_eq!(flags.join(","), "--namespace=prod");
		assert_eq!(pos.join(","), "api");
	}

	#[test]
	fn split_args_with_values_unknown_flag_falls_back_to_boolean() {
		// `--json` is not in `value_flags`, must not consume `next`.
		let args = vec!["--json".to_string(), "next".to_string()];
		let value_flags = vec!["namespace".to_string()];
		let (flags, pos) = split_args_with_values(&args, &value_flags);
		assert_eq!(flags.join(","), "--json");
		assert_eq!(pos.join(","), "next");
	}

	#[test]
	fn split_args_with_values_trailing_value_flag_without_value() {
		// Don't index past end — the flag parser will error later anyway.
		let args = vec!["api".to_string(), "--namespace".to_string()];
		let value_flags = vec!["namespace".to_string()];
		let (flags, pos) = split_args_with_values(&args, &value_flags);
		assert_eq!(flags.join(","), "--namespace");
		assert_eq!(pos.join(","), "api");
	}
}
