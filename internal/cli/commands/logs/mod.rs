//! `unitpm logs` — tail, follow, filter, and merge process log files.
//!
//! Module split:
//!
//!   - [`entry`]: line parsing, banner detection, batch readers.
//!   - [`merge`]: stable k-way merge by `(ts, seq)`; the bounded-tail
//!     reader that scans a file's last `n` lines from end-of-file.
//!   - [`follow`]: the follow loop — per-source tail goroutines feeding
//!     entries into a small-window heap, flushed after a short delay.
//!   - [`legacy`]: the pre-merge `--no-merge` path that tails every
//!     source independently and emits in arrival order.
//!   - [`guard`]: the size guard rails (`--all` / large `-n`).
//!   - [`args`]: the hand-rolled flag + target parser, the spec lookup,
//!     and the option → filter / source translation.
//!
//! Public entry point: [`run`].

mod args;
mod entry;
mod follow;
mod guard;
mod legacy;
mod merge;

use std::io::{self, Write};

use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::cli::root::cmd;
use crate::term;

use args::{build_filter, build_sources, parse_args, resolve_target};
use follow::Sleeper;

pub use args::Options;
pub use follow::{EntryHeap, FollowMessage, Sleeper as FollowSleeper, FLUSH_DELAY};
pub use guard::{format_bytes, guard_large_read, total_size, BLOCK_SIZE, WARN_SIZE};
pub use merge::{bounded_tail, stream_merge, Filter, StreamSource};

/// Default `--lines` value when the user does not pass a count.
pub const DEFAULT_LINES: usize = 40;

/// Run the `logs` command against `args`. Public entry point invoked by
/// the dispatcher; tests pass `args` directly.
pub fn run(args: &[String]) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	let opts = parse_args(args)?;
	let spec = resolve_target(&opts.target)?;
	let sources = build_sources(&spec, &opts);
	let filter = build_filter(&opts)?;

	let stdout = io::stdout();
	let out = stdout.lock();
	println!("Showing logs for {} ({})", spec.name, spec.id);
	for s in &sources {
		println!("{} {}", color_label_pub(&s.label), s.path);
	}
	println!();

	if opts.no_merge {
		drop(out);
		legacy::run_legacy_split(sources, opts.follow, std::thread::sleep as Sleeper)?;
		return Ok(());
	}

	if opts.all {
		// Drop the stdout lock before calling into guard so the user
		// sees the warning/prompt without buffering tricks.
		drop(out);
		if let Err(e) =
			guard::guard_large_read(&sources, opts.yes, io::stdin().lock(), is_tty_pub())
		{
			return Err(Box::<dyn std::error::Error + Send + Sync>::from(e));
		}
		let stdout = io::stdout();
		let mut out = stdout.lock();
		merge::stream_merge(&mut out, &filter, &sources)?;
		if !opts.follow {
			return Ok(());
		}
		drop(out);
		let stdout = io::stdout();
		let mut out = stdout.lock();
		return follow::follow_merge(&mut out, &filter, sources, std::thread::sleep as Sleeper)
			.map_err(|e| Box::<dyn std::error::Error + Send + Sync>::from(format!("follow: {e}")));
	}

	let stdout = io::stdout();
	let mut out = stdout.lock();
	merge::bounded_tail(&mut out, &sources, opts.lines, &filter)?;
	if !opts.follow {
		return Ok(());
	}
	drop(out);
	let stdout = io::stdout();
	let mut out = stdout.lock();
	follow::follow_merge(&mut out, &filter, sources, std::thread::sleep as Sleeper)
		.map_err(|e| Box::<dyn std::error::Error + Send + Sync>::from(format!("follow: {e}")))
}

fn color_label_pub(label: &str) -> String {
	match label {
		"STDOUT" => term::cyan(format_args!("[STDOUT]")),
		"STDERR" => term::red(format_args!("[STDERR]")),
		other => term::dim(format_args!("[{}]", other)),
	}
}

fn is_tty_pub() -> bool {
	term::is_tty()
}

/// `CommandSpec` returned to the dispatcher at registration time.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: cmd::LOGS.to_string(),
		aliases: vec!["log".to_string()],
		usage: format!(
			"unitpm {} <id|name> [-n N] [--all] [-f] [--since DUR] [--grep RE] [--stdout|--stderr] [--no-merge]",
			cmd::LOGS
		),
		description: "View and follow process logs (chronologically merged).".to_string(),
		options: vec![
			HelpOption {
				short: "-h".into(),
				long: "--help".into(),
				description: "Show this help message.".into(),
			},
			HelpOption {
				short: "-n".into(),
				long: "--lines".into(),
				description: "Number of tail lines to show (default 40).".into(),
			},
			HelpOption {
				short: "-f".into(),
				long: "--follow".into(),
				description: "Follow the log file (like tail -f).".into(),
			},
			HelpOption {
				short: "".into(),
				long: "--tail".into(),
				description: "Alias for --lines.".into(),
			},
			HelpOption {
				short: "".into(),
				long: "--all".into(),
				description: "Read the whole file (guarded).".into(),
			},
			HelpOption {
				short: "-y".into(),
				long: "--yes".into(),
				description: "Bypass the size guard.".into(),
			},
			HelpOption {
				short: "".into(),
				long: "--no-merge".into(),
				description: "Use the legacy per-stream tail.".into(),
			},
			HelpOption {
				short: "".into(),
				long: "--since".into(),
				description: "Show entries newer than this duration (e.g. 30m).".into(),
			},
			HelpOption {
				short: "-g".into(),
				long: "--grep".into(),
				description: "Regex filter on entry body.".into(),
			},
			HelpOption {
				short: "-o".into(),
				long: "--stdout".into(),
				description: "Include only stdout.".into(),
			},
			HelpOption {
				short: "-e".into(),
				long: "--stderr".into(),
				description: "Include only stderr.".into(),
			},
		],
		examples: vec![
			format!("unitpm {} api", cmd::LOGS),
			format!("unitpm {} api --follow", cmd::LOGS),
			format!("unitpm {} api --tail 100", cmd::LOGS),
			format!("unitpm {} api --all --grep ERROR", cmd::LOGS),
			format!("unitpm {} api --since 30m", cmd::LOGS),
			format!("unitpm {} prod:api", cmd::LOGS),
		],
		hidden: false,
	}
}

/// Print the command help block to `w`. Exposed so tests can capture it.
pub fn print_help<W: Write>(w: &mut W) -> io::Result<()> {
	crate::cli::help::render_command_help(w, &spec())
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn spec_includes_log_alias() {
		let s = spec();
		assert_eq!(s.name, "logs");
		assert!(s.aliases.contains(&"log".to_string()));
	}

	#[test]
	fn spec_help_renders() {
		let mut buf = Vec::new();
		print_help(&mut buf).unwrap();
		let plain = crate::cli::format::strip_ansi(&String::from_utf8(buf).unwrap());
		assert!(plain.contains("Usage:"));
		assert!(plain.contains("--follow"));
	}

	#[test]
	fn color_label_distinguishes_stdout_stderr() {
		let a = color_label_pub("STDOUT");
		let b = color_label_pub("STDERR");
		assert!(!a.is_empty() && !b.is_empty());
		assert_ne!(a, b);
	}

	#[test]
	fn format_bytes_export_works() {
		assert_eq!(format_bytes(1024), "1.0 KiB");
	}

	#[test]
	fn default_lines_constant() {
		assert_eq!(DEFAULT_LINES, 40);
	}
}
