//! Root command — dispatches every CLI invocation.
//!
//! 9 cases ported from `internal/cli/root/{root_test.go, root_internal_test.go}`.
//!
//! Phase 6a shells the 21 commands as stub `CommandSpec`s so the dispatcher,
//! help renderer, registry, and global-flag wiring can be exercised and
//! frozen before phase 6b lands each real implementation. Phase 6d wired
//! the 14 commands it covered; the remaining stubs are listed in
//! [`stubs::STUB_COMMANDS`].
//!
//! `apply_global_flags` recognises `--quiet` / `-q` and toggles the global
//! quiet state on [`crate::term`].
//!
//! The writer-aware variants ([`execute_with`], [`print_command_help_to`],
//! [`handle_error_to`]) exist so tests can sink the side-effect output;
//! the global [`execute`] / [`print_command_help`] / [`handle_error`]
//! functions are the ones the binary entry point calls.

mod stubs;

pub use stubs::{cmd, has_help, is_stubbed, not_yet_ported, register_all, stub_spec};

use std::io::{self, Write};

use crate::cli::commands;
use crate::cli::errs::UsageError;
use crate::cli::help::render_root_help;
use crate::cli::registry;
use crate::term;

/// Dispatch `args` to the matching command. Writes help to stdout and
/// errors to stderr. Returns the process exit code.
pub fn execute(args: &[String]) -> i32 {
	let stdout = io::stdout();
	let stderr = io::stderr();
	let mut out = stdout.lock();
	let mut err = stderr.lock();
	execute_with(args, &mut out, &mut err)
}

/// Writer-bound dispatcher. Tests pass silent writers; production
/// callers go through [`execute`].
pub fn execute_with<O: Write, E: Write>(args: &[String], out: &mut O, err: &mut E) -> i32 {
	register_all();
	let specs = registry::get_all();

	let args = apply_global_flags(args);

	if args.is_empty() {
		let _ = render_root_help(out, &specs, true);
		return 0;
	}

	let cmd_name = match resolve_command(&args[0]) {
		Some(n) => n,
		None => {
			print_error(err, &format!("Command not found: {}", args[0]));
			let _ = render_root_help(out, &specs, true);
			return 1;
		}
	};

	if cmd_name == "-h" || cmd_name == "--help" {
		let _ = render_root_help(out, &specs, true);
		return 0;
	}

	if cmd_name == cmd::HELP {
		if args.len() > 1 {
			match resolve_command(&args[1]) {
				Some(sub) if sub != cmd::HELP => return print_command_help_to(&sub, out),
				Some(sub) if sub == cmd::HELP => {
					let _ = render_root_help(out, &specs, true);
					return 0;
				}
				_ => {
					print_error(err, &format!("Command not found: {}", args[1]));
					let _ = render_root_help(out, &specs, true);
					return 1;
				}
			}
		}
		let _ = render_root_help(out, &specs, true);
		return 0;
	}

	// `command --help` → print command-specific help.
	if args.len() > 1 && is_help_request(&args[1..]) {
		return print_command_help_to(&cmd_name, out);
	}

	match dispatch(&cmd_name, &args[1..], out, err) {
		DispatchOutcome::Ok => 0,
		DispatchOutcome::Unknown => {
			print_error(err, &format!("Command not found: {cmd_name}"));
			1
		}
		DispatchOutcome::Err(cmd_err) => {
			handle_error_to(cmd_err, &cmd_name, out, err);
			1
		}
	}
}

/// Outcome of [`dispatch`]. `Ok` exits 0; `Unknown` means the command
/// name was recognised by the registry but had no implementation
/// (yet); `Err` is a real command-level error.
enum DispatchOutcome {
	Ok,
	Unknown,
	Err(Box<dyn std::error::Error>),
}

/// Hand `name` to the right command module. Phase 6d covers the
/// `apply`, `completion`, `delete`, `execenv`/`execsandbox`, `export`,
/// `flush`, `install-tools`, `reload`, `reset`, `scale`, `startup`,
/// `update`, and `version` commands. Everything else falls back to a
/// stub.
fn dispatch<O: Write, E: Write>(
	name: &str,
	args: &[String],
	out: &mut O,
	err: &mut E,
) -> DispatchOutcome {
	match name {
		cmd::APPLY => commands::apply::run(None, out, args).into(),
		cmd::COMPLETION => commands::completion::run(out, args).into(),
		cmd::DELETE => commands::delete::run(None, out, args).into(),
		cmd::EXEC_ENV => commands::execenv::run(err, args).into(),
		cmd::EXEC_SANDBOX => commands::execsandbox::run(out, args).into(),
		cmd::EXPORT => commands::export::run(out, args).into(),
		cmd::FLUSH => commands::flush::run(None, out, args).into(),
		cmd::INSTALL_TOOLS => commands::installtools::run(out, args, None).into(),
		cmd::RELOAD => commands::reload::run(None, out, args).into(),
		cmd::RESET => commands::reset::run(None, out, args).into(),
		cmd::SCALE => commands::scale::run(None, out, args).into(),
		cmd::STARTUP => commands::startup::run(&mut commands::startup::RealRunner, args).into(),
		cmd::UPDATE => commands::update::run(out, args).into(),
		cmd::VERSION => commands::version::run(None, out, args).into(),
		_ if is_stubbed(name) => DispatchOutcome::Err(not_yet_ported(name)),
		_ => DispatchOutcome::Unknown,
	}
}

impl From<Result<(), Box<dyn std::error::Error>>> for DispatchOutcome {
	fn from(r: Result<(), Box<dyn std::error::Error>>) -> Self {
		match r {
			Ok(()) => DispatchOutcome::Ok,
			Err(e) => DispatchOutcome::Err(e),
		}
	}
}

/// Resolve a command-name argument to the canonical command name. Mirrors
/// the Go: `help` is mapped explicitly, `--version` becomes `version`, the
/// help flags pass through, and everything else goes through the registry.
#[must_use]
pub fn resolve_command(name: &str) -> Option<String> {
	if name == cmd::HELP {
		return Some(cmd::HELP.to_string());
	}
	if name == "--version" {
		return Some(cmd::VERSION.to_string());
	}
	if name == "-h" || name == "--help" {
		return Some(name.to_string());
	}
	let (canonical, hit) = registry::resolve(name);
	if hit {
		Some(canonical)
	} else {
		None
	}
}

/// Print the command-specific help block for `name` to stdout. Returns 0
/// in every case — the Go tests pin this for both known and unknown names,
/// and an unknown command does not crash here (the dispatcher handles the
/// "not found" message earlier in the flow).
#[must_use]
pub fn print_command_help(name: &str) -> i32 {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	print_command_help_to(name, &mut out)
}

/// Writer-bound variant of [`print_command_help`].
#[must_use]
pub fn print_command_help_to<W: Write>(name: &str, out: &mut W) -> i32 {
	let spec = match command_spec(name) {
		Some(s) => s,
		None => stub_spec(name),
	};
	let _ = crate::cli::help::render_command_help(out, &spec);
	0
}

/// Look up a real command spec by name. Returns `None` when the name
/// is still on the stub roster so the caller can fall back.
#[must_use]
pub fn command_spec(name: &str) -> Option<crate::cli::help::CommandSpec> {
	match name {
		cmd::APPLY => Some(commands::apply::spec()),
		cmd::COMPLETION => Some(commands::completion::spec()),
		cmd::DELETE => Some(commands::delete::spec()),
		cmd::EXEC_ENV => Some(commands::execenv::spec()),
		cmd::EXEC_SANDBOX => Some(commands::execsandbox::spec()),
		cmd::EXPORT => Some(commands::export::spec()),
		cmd::FLUSH => Some(commands::flush::spec()),
		cmd::INSTALL_TOOLS => Some(commands::installtools::spec()),
		cmd::RELOAD => Some(commands::reload::spec()),
		cmd::RESET => Some(commands::reset::spec()),
		cmd::SCALE => Some(commands::scale::spec()),
		cmd::STARTUP => Some(commands::startup::spec()),
		cmd::UPDATE => Some(commands::update::spec()),
		cmd::VERSION => Some(commands::version::spec()),
		_ => None,
	}
}

/// Decide what to do with an error from a subcommand. Usage errors get a
/// command-specific help dump appended; everything else prints the raw
/// message. Writes to stderr and stdout.
pub fn handle_error(err: Box<dyn std::error::Error>, cmd_name: &str) {
	let stdout = io::stdout();
	let stderr = io::stderr();
	let mut out = stdout.lock();
	let mut err_out = stderr.lock();
	handle_error_to(err, cmd_name, &mut out, &mut err_out)
}

/// Writer-bound variant of [`handle_error`].
pub fn handle_error_to<O: Write, E: Write>(
	err: Box<dyn std::error::Error>,
	cmd_name: &str,
	out: &mut O,
	err_out: &mut E,
) {
	if let Some(usage) = err.downcast_ref::<UsageError>() {
		let _ = writeln!(
			err_out,
			"{}",
			term::red(format_args!("[unitpm][ERROR] {usage}"))
		);
	} else {
		let _ = writeln!(
			err_out,
			"{}",
			term::red(format_args!("[unitpm][ERROR] {err}"))
		);
	}
	let _ = print_command_help_to(cmd_name, out);
}

/// Returns true when any token in `args` is `-h` or `--help`.
#[must_use]
pub fn is_help_request(args: &[String]) -> bool {
	args.iter().any(|a| a == "-h" || a == "--help")
}

/// Strip recognised global flags (`--quiet`, `-q`) from `args` and apply
/// them as side effects. The Go side lives in `applyGlobalFlags`.
#[must_use]
pub fn apply_global_flags(args: &[String]) -> Vec<String> {
	let mut out = Vec::with_capacity(args.len());
	for a in args {
		if a == "--quiet" || a == "-q" {
			term::set_quiet(true);
			continue;
		}
		out.push(a.clone());
	}
	out
}

fn print_error<W: Write>(w: &mut W, msg: &str) {
	let _ = writeln!(w, "{}", term::red(format_args!("[unitpm][ERROR] {msg}")));
}

#[cfg(test)]
mod tests;
