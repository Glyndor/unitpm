//! Root command — dispatches every CLI invocation.
//!
//! 9 cases ported from `internal/cli/root/{root_test.go, root_internal_test.go}`.
//!
//! Phase 6a shells the 21 commands as stub `CommandSpec`s so the dispatcher,
//! help renderer, registry, and global-flag wiring can be exercised and
//! frozen before phase 6b lands each real implementation. Every stub
//! reports a "not yet ported" error so callers see the same exit-code
//! surface as the eventual real implementation.
//!
//! `apply_global_flags` recognises `--quiet` / `-q` and toggles the global
//! quiet state on [`crate::term`].
//!
//! The writer-aware variants ([`execute_with`], [`print_command_help_to`],
//! [`handle_error_to`]) exist so tests can sink the side-effect output;
//! the global [`execute`] / [`print_command_help`] / [`handle_error`]
//! functions are the ones the binary entry point calls.

mod stubs;

pub use stubs::{cmd, has_help, is_stubbed, not_yet_ported, register_all, run_dispatch, stub_spec};

use std::io::{self, Write};

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

	if is_stubbed(&cmd_name) {
		// Real lifecycle commands ported in phase 6b — wire them
		// through to their typed `run`. Other names get the
		// "not yet ported" path.
		if matches!(
			cmd_name.as_str(),
			stubs::cmd::LIST | stubs::cmd::START | stubs::cmd::STOP | stubs::cmd::RESTART
		) {
			return run_real(&cmd_name, &args[1..], out, err);
		}
		if let Some(cmd_err) = run_command(&cmd_name, &args[1..]) {
			handle_error_to(cmd_err, &cmd_name, out, err);
			return 1;
		}
		return 0;
	}

	// Unknown command.
	print_error(err, &format!("Command not found: {}", cmd_name));
	let _ = render_root_help(out, &specs, true);
	1
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

/// Run the four real phase-6b lifecycle commands.
///
/// Each typed `run` needs an IPC client. The dispatcher falls back to
/// a "connection refused" error when no client is available — the same
/// behaviour the Go side surfaces through `transport.NewClient()`'s
/// `DaemonUnreachable` variant. Production callers wire a real client
/// ahead of [`execute_with`] by overriding [`stubs::set_dispatcher_client`].
fn run_real<O: Write, E: Write>(name: &str, args: &[String], out: &mut O, err: &mut E) -> i32 {
	let no_client_msg = || -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!(
			"{name}: daemon unreachable (no IPC client wired into the dispatcher)"
		))
	};
	let mut client = match stubs::take_dispatcher_client() {
		Some(c) => c,
		None => {
			handle_error_to(no_client_msg(), name, out, err);
			return 1;
		}
	};
	let result = match name {
		stubs::cmd::LIST => {
			let mut handle = client.list_handle();
			crate::cli::commands::list::run(&mut handle, out, args)
				.map_err(|e| -> Box<dyn std::error::Error> { e })
		}
		stubs::cmd::START => {
			let mut handle = client.start_handle();
			crate::cli::commands::start::run::<O, E, _>(Some(&mut handle), out, err, args)
				.map_err(|e| -> Box<dyn std::error::Error> { e })
		}
		stubs::cmd::STOP => {
			let mut handle = client.stop_handle();
			crate::cli::commands::stop::run(&mut handle, out, err, args)
				.map_err(|e| -> Box<dyn std::error::Error> { e })
		}
		stubs::cmd::RESTART => {
			let mut handle = client.restart_handle();
			crate::cli::commands::restart::run(&mut handle, out, err, args)
				.map_err(|e| -> Box<dyn std::error::Error> { e })
		}
		_ => unreachable!("run_real invoked with non-6b command name"),
	};
	// Client ownership ends on return; we used it by-reference above.
	drop(client);
	match result {
		Ok(()) => 0,
		Err(e) => {
			handle_error_to(e, name, out, err);
			1
		}
	}
}

/// Invoke `name` with `args`. Returns `Some(err)` on a command error;
/// `None` means "command ran fine or wasn't recognised" — the Go side
/// returns `nil` for unknown names, and the test
/// `TestRunCommand_UnknownReturnsNil` pins that.
///
/// Phase 6c forwards `logs` / `monit` / `show` to their real entry
/// points through [`run_dispatch`] before falling back to the stub.
#[must_use]
pub fn run_command(name: &str, args: &[String]) -> Option<Box<dyn std::error::Error>> {
	if let Some(err) = run_dispatch(name, args) {
		return Some(err);
	}
	if is_stubbed(name) {
		Some(not_yet_ported(name))
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
	if has_help(name) {
		let spec = stub_spec(name);
		let _ = crate::cli::help::render_command_help(out, &spec);
	}
	0
}

/// Decide what to do with an error from a subcommand. Usage errors get a
/// command-specific help dump appended; everything else prints the raw
/// message. Writes to stderr and stdout.
pub fn handle_error(err: Box<dyn std::error::Error>, cmd_name: &str) {
	let stdout = io::stdout();
	let stderr = io::stderr();
	let mut out = stdout.lock();
	let mut err_out = stderr.lock();
	handle_error_to(err, cmd_name, &mut out, &mut err_out);
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
mod tests {
	use super::*;

	#[test]
	fn execute_help_variants_return_zero() {
		// Sink the side-effect output so the test runner stays clean.
		let mut out = Vec::new();
		let mut err = Vec::new();
		assert_eq!(execute_with(&["help".into()], &mut out, &mut err), 0);
		assert_eq!(execute_with(&["--help".into()], &mut out, &mut err), 0);
		assert_eq!(execute_with(&["-h".into()], &mut out, &mut err), 0);
	}

	#[test]
	fn execute_unknown_command_returns_one() {
		let mut out = Vec::new();
		let mut err = Vec::new();
		assert_eq!(
			execute_with(&["unknown-command".into()], &mut out, &mut err),
			1
		);
		assert!(
			!err.is_empty(),
			"unknown-command should have written to stderr"
		);
	}

	#[test]
	fn is_help_request_true_for_recognised_flags() {
		let cases: &[&[&str]] = &[
			&["-h"],
			&["--help"],
			&["start", "-h"],
			&["--help", "something"],
			&["foo", "--help", "bar"],
		];
		for case in cases {
			let args: Vec<String> = case.iter().map(|s| (*s).to_string()).collect();
			assert!(
				is_help_request(&args),
				"is_help_request({args:?}) should be true"
			);
		}
	}

	#[test]
	fn is_help_request_false_for_non_help_args() {
		let cases: &[&[&str]] = &[
			&[],
			&["start"],
			&["start", "--name", "api"],
			&["-help"],
			&["help"],
		];
		for case in cases {
			let args: Vec<String> = case.iter().map(|s| (*s).to_string()).collect();
			assert!(
				!is_help_request(&args),
				"is_help_request({args:?}) should be false"
			);
		}
	}

	#[test]
	fn handle_error_usage_error_does_not_panic() {
		// Render the usage path; output goes to a sink buffer which the test
		// does not assert on. Avoids polluting the test runner's stdout.
		let err: Box<dyn std::error::Error> =
			Box::new(UsageError::new("missing required flag --name"));
		let mut out = Vec::new();
		let mut err_out = Vec::new();
		handle_error_to(err, "start", &mut out, &mut err_out);
	}

	#[test]
	fn handle_error_generic_error_does_not_panic() {
		#[derive(Debug)]
		struct TestError(&'static str);
		impl std::fmt::Display for TestError {
			fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
				f.write_str(self.0)
			}
		}
		impl std::error::Error for TestError {}
		let err: Box<dyn std::error::Error> = Box::new(TestError("daemon not running"));
		let mut out = Vec::new();
		let mut err_out = Vec::new();
		handle_error_to(err, "list", &mut out, &mut err_out);
	}

	#[test]
	fn print_command_help_unknown_returns_zero() {
		let mut buf = Vec::new();
		assert_eq!(print_command_help_to("unknown-xyz-command", &mut buf), 0);
	}

	#[test]
	fn print_command_help_known_returns_zero() {
		let known = [
			cmd::LIST,
			cmd::START,
			cmd::STOP,
			cmd::RESTART,
			cmd::DELETE,
			cmd::LOGS,
			cmd::VERSION,
		];
		for name in known {
			let mut buf = Vec::new();
			assert_eq!(
				print_command_help_to(name, &mut buf),
				0,
				"{name} returned non-zero"
			);
		}
	}

	#[test]
	fn run_command_unknown_returns_none() {
		// Unknown command: must return `None` (matches Go's `nil`) so the
		// dispatcher doesn't surface a phantom error.
		let err = run_command("nonexistent-command-xyz", &[]);
		assert!(err.is_none(), "expected None, got {err:?}");
	}
}
