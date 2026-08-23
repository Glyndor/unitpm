//! Terminal styling, color, and capability detection.
//!
//! The terminal-width probe lives in [`size`]; it is split out so the colour
//! module stays under the file-size cap and because the probe is logically
//! independent of colour.

mod size;

pub use size::get_terminal_width;

use std::io::{self, Write};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::OnceLock;

/// Resets the terminal styling.
pub const RESET: &str = "\x1b[0m";
/// Bold styling.
pub const BOLD: &str = "\x1b[1m";
/// Dim styling.
pub const DIM: &str = "\x1b[2m";
/// Red foreground.
pub const RED: &str = "\x1b[31m";
/// Green foreground.
pub const GREEN: &str = "\x1b[32m";
/// Yellow foreground.
pub const YELLOW: &str = "\x1b[33m";
/// Blue foreground.
pub const BLUE: &str = "\x1b[34m";
/// Magenta foreground.
pub const MAGENTA: &str = "\x1b[35m";
/// Cyan foreground.
pub const CYAN: &str = "\x1b[36m";
/// Gray foreground.
pub const GRAY: &str = "\x1b[37m";

/// Renders ANSI colour codes, gated on the current terminal's capability.
#[derive(Debug, Clone, Copy)]
pub struct Styler {
	enabled: bool,
}

impl Styler {
	/// Detect the current terminal's colour support and freeze the decision.
	#[must_use]
	pub fn new() -> Self {
		Self {
			enabled: should_use_color(),
		}
	}

	/// Force a specific enabled state. Tests use this to exercise the two
	/// branches without depending on the ambient terminal.
	#[must_use]
	pub fn with_enabled(enabled: bool) -> Self {
		Self { enabled }
	}

	/// Whether the styler currently emits escape codes.
	#[must_use]
	pub fn enabled(self) -> bool {
		self.enabled
	}

	/// Wrap `text` in `code ... RESET` when colour is on; otherwise pass it
	/// through unchanged.
	#[must_use]
	pub fn colorize(self, code: &str, text: &str) -> String {
		if self.enabled {
			format!("{code}{text}{RESET}")
		} else {
			text.to_string()
		}
	}

	/// Format `args` in red via this styler.
	pub fn red(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(RED, &format!("{args}"))
	}

	/// Format `args` in green via this styler.
	pub fn green(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(GREEN, &format!("{args}"))
	}

	/// Format `args` in yellow via this styler.
	pub fn yellow(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(YELLOW, &format!("{args}"))
	}

	/// Format `args` in blue via this styler.
	pub fn blue(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(BLUE, &format!("{args}"))
	}

	/// Format `args` in cyan via this styler.
	pub fn cyan(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(CYAN, &format!("{args}"))
	}

	/// Format `args` in magenta via this styler.
	pub fn magenta(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(MAGENTA, &format!("{args}"))
	}

	/// Format `args` in bold via this styler.
	pub fn bold(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(BOLD, &format!("{args}"))
	}

	/// Format `args` in dim via this styler.
	pub fn dim(self, args: fmt::Arguments<'_>) -> String {
		self.colorize(DIM, &format!("{args}"))
	}
}

impl Default for Styler {
	fn default() -> Self {
		Self::new()
	}
}

fn std_styler() -> &'static Styler {
	static STD: OnceLock<Styler> = OnceLock::new();
	STD.get_or_init(Styler::new)
}

/// Format `args` in red via the default styler.
#[must_use]
pub fn red(args: fmt::Arguments<'_>) -> String {
	std_styler().red(args)
}

/// Format `args` in green via the default styler.
#[must_use]
pub fn green(args: fmt::Arguments<'_>) -> String {
	std_styler().green(args)
}

/// Format `args` in yellow via the default styler.
#[must_use]
pub fn yellow(args: fmt::Arguments<'_>) -> String {
	std_styler().yellow(args)
}

/// Format `args` in blue via the default styler.
#[must_use]
pub fn blue(args: fmt::Arguments<'_>) -> String {
	std_styler().blue(args)
}

/// Format `args` in cyan via the default styler.
#[must_use]
pub fn cyan(args: fmt::Arguments<'_>) -> String {
	std_styler().cyan(args)
}

/// Format `args` in magenta via the default styler.
#[must_use]
pub fn magenta(args: fmt::Arguments<'_>) -> String {
	std_styler().magenta(args)
}

/// Format `args` in bold via the default styler.
#[must_use]
pub fn bold(args: fmt::Arguments<'_>) -> String {
	std_styler().bold(args)
}

/// Format `args` in dim via the default styler.
#[must_use]
pub fn dim(args: fmt::Arguments<'_>) -> String {
	std_styler().dim(args)
}

/// Write `args` to `w`, suppressed when quiet mode is active.
///
/// This is the writer-taking core used by the batch module's summary line
/// and analogous per-target messages. Returning `Ok(())` on quiet keeps the
/// caller from having to branch.
pub fn printf<W: Write>(w: &mut W, args: fmt::Arguments<'_>) -> io::Result<()> {
	if is_quiet() {
		return Ok(());
	}
	w.write_fmt(args)
}

/// Write `args` to `w` followed by a newline, suppressed when quiet.
pub fn println<W: Write>(w: &mut W, args: fmt::Arguments<'_>) -> io::Result<()> {
	if is_quiet() {
		return Ok(());
	}
	w.write_fmt(args)?;
	w.write_all(b"\n")
}

static QUIET: AtomicBool = AtomicBool::new(false);

/// Toggle suppression of success/info messages. Errors are still surfaced.
pub fn set_quiet(q: bool) {
	QUIET.store(q, Ordering::Relaxed);
}

/// Report whether quiet mode is currently active.
#[must_use]
pub fn is_quiet() -> bool {
	QUIET.load(Ordering::Relaxed)
}

/// Whether stdout is attached to a terminal. On non-unix platforms this is a
/// conservative `false` — the only place we ever branch on it is the colour
/// gate, which the user can already override via `NO_COLOR`.
#[must_use]
pub fn is_tty() -> bool {
	#[cfg(unix)]
	{
		use std::os::unix::io::AsRawFd;
		let fd = std::io::stdout().as_raw_fd();
		// SAFETY: `isatty` is a pure query that does not mutate the fd.
		unsafe { libc::isatty(fd) != 0 }
	}
	#[cfg(not(unix))]
	{
		let _ = std::io::stdout();
		false
	}
}

/// Whether ANSI colour should be emitted.
///
/// Mirrors the Go precedence exactly: a non-TTY disables colour, `NO_COLOR`
/// (set to any value) disables colour, `TERM=dumb` disables colour, and an
/// empty `TERM` disables colour on Unix. Anything else is on.
#[must_use]
pub fn should_use_color() -> bool {
	if !is_tty() {
		return false;
	}
	if std::env::var_os("NO_COLOR").is_some() {
		return false;
	}
	let term = std::env::var("TERM").unwrap_or_default();
	if term == "dumb" {
		return false;
	}
	#[cfg(unix)]
	{
		!term.is_empty()
	}
	#[cfg(not(unix))]
	{
		true
	}
}

use std::fmt;

#[cfg(test)]
mod tests;
