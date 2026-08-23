//! Tests for the term module — 8 cases mirroring `internal/term/color_test.go`.

use std::sync::{Mutex, MutexGuard};

use super::*;

/// Process-global state in this module is the `QUIET` flag and the
/// `NO_COLOR` / `TERM` environment variables. `cargo test` parallelises by
/// default, and each phase that touched these has already shipped a race —
/// the gap was always **restoring**, never **serialising**. The guard below
/// holds a process-wide mutex so concurrent tests cannot see each other's
/// edits and restores every piece of global state in `Drop`, so a failing
/// assertion cannot leave the next test running against the previous one's
/// environment.
pub struct TermGuard {
	_lock: MutexGuard<'static, ()>,
	prev_quiet: bool,
	prev_no_color: Option<String>,
	prev_term: Option<String>,
}

static TERM_LOCK: Mutex<()> = Mutex::new(());

/// Acquire the term-state lock and capture a snapshot of the current
/// `quiet` flag plus the env vars the colour gate reads. Returns a
/// guard that restores everything on `Drop`. Re-exported for the
/// command-test modules in phase 6d.
pub fn lock_term() -> TermGuard {
	TermGuard {
		_lock: TERM_LOCK.lock().unwrap_or_else(|e| e.into_inner()),
		prev_quiet: is_quiet(),
		prev_no_color: std::env::var("NO_COLOR").ok(),
		prev_term: std::env::var("TERM").ok(),
	}
}

impl Drop for TermGuard {
	fn drop(&mut self) {
		set_quiet(self.prev_quiet);
		match self.prev_no_color.as_deref() {
			Some(v) => std::env::set_var("NO_COLOR", v),
			None => std::env::remove_var("NO_COLOR"),
		}
		match self.prev_term.as_deref() {
			Some(v) => std::env::set_var("TERM", v),
			None => std::env::remove_var("TERM"),
		}
	}
}

#[test]
fn color_string_helpers() {
	// Mirrors `TestColorStringHelpers` — exercises each package-level
	// helper once and asserts the formatted payload survives.
	let helpers: [fn(fmt::Arguments<'_>) -> String; 8] =
		[red, green, yellow, blue, cyan, magenta, bold, dim];
	for h in helpers {
		let got = h(format_args!("hello {}\n", "world"));
		assert!(
			got.contains("hello world"),
			"color helper dropped substring: {got:?}"
		);
	}
}

#[test]
fn styler_methods() {
	let s = Styler::with_enabled(false); // colour-off branch is the deterministic one
	let methods: [fn(Styler, fmt::Arguments<'_>) -> String; 8] = [
		Styler::red,
		Styler::green,
		Styler::yellow,
		Styler::blue,
		Styler::cyan,
		Styler::magenta,
		Styler::bold,
		Styler::dim,
	];
	for m in methods {
		let got = m(s, format_args!("x {}", 1));
		assert!(got.contains("x 1"), "styler dropped format args: {got:?}");
	}
}

#[test]
fn styler_enabled_reflects_construction() {
	let on = Styler::with_enabled(true);
	let off = Styler::with_enabled(false);
	assert!(on.enabled(), "with_enabled(true) should report enabled");
	assert!(!off.enabled(), "with_enabled(false) should report disabled");
}

#[test]
fn styler_colorize_disabled() {
	let s = Styler::with_enabled(false);
	let got = s.colorize(RED, "x");
	assert_eq!(
		got, "x",
		"disabled styler must not emit escapes, got {got:?}"
	);
}

#[test]
fn styler_colorize_enabled() {
	let s = Styler::with_enabled(true);
	let got = s.colorize(RED, "x");
	assert!(got.contains(RED), "expected red opener, got {got:?}");
	assert!(got.contains("x"), "expected payload, got {got:?}");
	assert!(got.contains(RESET), "expected reset, got {got:?}");
}

#[test]
fn printf_and_println_write_to_writer() {
	let _g = lock_term();
	set_quiet(false);

	let mut buf = Vec::new();
	printf(&mut buf, format_args!("hello {}\n", "x")).expect("printf");
	println(&mut buf, format_args!("{}", "bye")).expect("println");

	let out = String::from_utf8(buf).expect("utf8");
	assert!(out.contains("hello x"), "missing printf payload: {out:?}");
	assert!(out.contains("bye"), "missing println payload: {out:?}");
}

#[test]
fn set_quiet_suppresses_writer_output() {
	let _g = lock_term();
	set_quiet(true);

	let mut buf = Vec::new();
	printf(&mut buf, format_args!("should-not-appear\n")).expect("printf");
	println(&mut buf, format_args!("also-suppressed")).expect("println");

	assert!(
		buf.is_empty(),
		"quiet mode should swallow output, got {:?}",
		String::from_utf8_lossy(&buf)
	);
}

#[test]
fn is_tty_and_should_use_color_are_safe_under_tests() {
	// Under `cargo test` stdout is a pipe → not a TTY → no colour. We do not
	// assert the exact value because the colour decision also depends on
	// TERM and NO_COLOR; both are restored by `lock_term`. The point of this
	// case is just to make sure neither function panics on a piped stdout.
	let _g = lock_term();
	let _ = is_tty();
	let _ = should_use_color();
}

/// Sanity test for the helper used by the batch module — the ANSI escapes
/// it relies on must round-trip through `printf` unchanged so the writer
/// gets the same bytes the user would see in a real terminal.
#[test]
fn printf_passes_through_ansi_escapes() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let payload = format_args!("\x1b[31mred\x1b[0m");
	printf(&mut buf, payload).expect("printf");
	assert_eq!(buf, b"\x1b[31mred\x1b[0m");
}
