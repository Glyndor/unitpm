//! Per-command stub registry.
//!
//! Phase 6a ships no real commands; every command the root dispatcher
//! recognises has a stub spec here that produces a "not yet ported"
//! error. Phase 6c replaces the `logs`, `monit`, and `show` stubs with
//! real registrations + dispatchers; the rest still report
//! "not yet ported" until phases 6b / 6d land.
//!
//! Phase 7a fills in the production dispatcher client
//! ([`transport_dispatcher_client`]) so `unitpm list` actually dials the
//! socket instead of answering "no IPC client wired into the
//! dispatcher". The four backend trait impls live at the bottom of the
//! file; the struct itself wraps `transport::Client` behind an
//! `Arc<Mutex<..>>` so each `*_handle()` can hand out an owned handle
//! without moving the original client.

use crate::cli::commands::list;
use crate::cli::commands::logs;
use crate::cli::commands::monit;
use crate::cli::commands::restart;
use crate::cli::commands::show;
use crate::cli::commands::start;
use crate::cli::commands::stop;
use crate::cli::help::CommandSpec;
use crate::cli::registry;

/// Names recognised by the dispatcher. Kept as constants so a rename only
/// happens in one place.
pub mod cmd {
	pub const LIST: &str = "list";
	pub const LOGS: &str = "logs";
	pub const START: &str = "start";
	pub const STOP: &str = "stop";
	pub const RESTART: &str = "restart";
	pub const DELETE: &str = "delete";
	pub const STARTUP: &str = "startup";
	pub const VERSION: &str = "version";
	pub const APPLY: &str = "apply";
	pub const EXPORT: &str = "export";
	pub const SHOW: &str = "show";
	pub const MONIT: &str = "monit";
	pub const RELOAD: &str = "reload";
	pub const RESET: &str = "reset";
	pub const SCALE: &str = "scale";
	pub const FLUSH: &str = "flush";
	pub const UPDATE: &str = "update";
	pub const INSTALL_TOOLS: &str = "install-tools";
	pub const EXEC_ENV: &str = "_exec-env";
	pub const EXEC_SANDBOX: &str = "_exec-sandbox";
	pub const COMPLETION: &str = "completion";
	pub const HELP: &str = "help";
}

/// Commands that have a real implementation in this phase. Used by
/// [`register_all`] to register the real spec and by [`run_dispatch`]
/// to forward to the real entry point. Everything else still goes
/// through the phase 6a stub path.
/// Commands that have a real implementation rather than a phase 6a stub.
///
/// Phases 6b, 6c and 6d ported the command tree in parallel, so this list is
/// where the lanes meet. It grows as each lands; when it holds every command,
/// the stub machinery in this file goes away entirely.
const PORTED: &[&str] = &[
	// 6b — lifecycle
	cmd::LIST,
	cmd::START,
	cmd::STOP,
	cmd::RESTART,
	// 6c — output
	cmd::LOGS,
	cmd::SHOW,
	cmd::MONIT,
	// 6d — the rest
	cmd::APPLY,
	cmd::COMPLETION,
	cmd::DELETE,
	cmd::EXEC_ENV,
	cmd::EXEC_SANDBOX,
	cmd::EXPORT,
	cmd::FLUSH,
	cmd::INSTALL_TOOLS,
	cmd::RELOAD,
	cmd::RESET,
	cmd::SCALE,
	cmd::STARTUP,
	cmd::UPDATE,
	cmd::VERSION,
];

/// Standard error returned by every stub command. Phase 6b replaces these
/// with real implementations; the message is intentionally descriptive so a
/// user who somehow runs a 6a build on a command the dispatcher already
/// recognises knows what is going on.
#[must_use]
pub fn not_yet_ported(name: &str) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(format!(
		"unitpm {name}: not yet ported (phase 6a only ships the CLI infrastructure)"
	))
}

/// Stub `CommandSpec` for `name`. Every entry has the same shape — usage
/// is `unitpm <name>`, description notes the phase 6a status, and the two
/// hidden internal wrappers (`_exec-env`, `_exec-sandbox`) keep their
/// `hidden = true` so they stay out of the root help page.
#[must_use]
pub fn stub_spec(name: &str) -> CommandSpec {
	let usage = format!("unitpm {name}");
	let description =
		format!("{name} is part of the phase 6b–6d command port — not yet implemented.");
	CommandSpec {
		name: name.to_string(),
		aliases: Vec::new(),
		usage,
		description,
		options: Vec::new(),
		examples: Vec::new(),
		hidden: matches!(name, cmd::EXEC_ENV | cmd::EXEC_SANDBOX),
	}
}

/// Register every spec with the global registry. Phase 6c commands get
/// the real spec from their own module; the rest get the phase 6a
/// stub. Called by the dispatcher at the top of `execute` so a fresh
/// process always sees a fully-populated registry.
pub fn register_all() {
	let names = [
		cmd::LIST,
		cmd::LOGS,
		cmd::START,
		cmd::STOP,
		cmd::RESTART,
		cmd::DELETE,
		cmd::STARTUP,
		cmd::VERSION,
		cmd::UPDATE,
		cmd::INSTALL_TOOLS,
		cmd::EXEC_ENV,
		cmd::EXEC_SANDBOX,
		cmd::COMPLETION,
		cmd::APPLY,
		cmd::EXPORT,
		cmd::SHOW,
		cmd::MONIT,
		cmd::RELOAD,
		cmd::RESET,
		cmd::SCALE,
		cmd::FLUSH,
	];
	for n in names {
		if let Some(spec) = ported_spec(n) {
			registry::register(spec);
		} else {
			registry::register(stub_spec(n));
		}
	}
}

/// Real `CommandSpec` for a ported command. `None` means "no real spec yet"
/// and the caller falls back to the phase 6a stub.
///
/// Every arm here must have its name in [`PORTED`], or the command registers
/// its real help page while `run_command` still reports it as unported — which
/// is the state phase 6b's four commands were left in.
fn ported_spec(name: &str) -> Option<CommandSpec> {
	match name {
		cmd::LIST => Some(list::spec()),
		cmd::START => Some(start::spec()),
		cmd::STOP => Some(stop::spec()),
		cmd::RESTART => Some(restart::spec()),
		cmd::LOGS => Some(logs::spec()),
		cmd::SHOW => Some(show::spec()),
		cmd::MONIT => Some(monit::spec()),
		cmd::APPLY => Some(crate::cli::commands::apply::spec()),
		cmd::COMPLETION => Some(crate::cli::commands::completion::spec()),
		cmd::DELETE => Some(crate::cli::commands::delete::spec()),
		cmd::EXEC_ENV => Some(crate::cli::commands::execenv::spec()),
		cmd::EXEC_SANDBOX => Some(crate::cli::commands::execsandbox::spec()),
		cmd::EXPORT => Some(crate::cli::commands::export::spec()),
		cmd::FLUSH => Some(crate::cli::commands::flush::spec()),
		cmd::INSTALL_TOOLS => Some(crate::cli::commands::installtools::spec()),
		cmd::RELOAD => Some(crate::cli::commands::reload::spec()),
		cmd::RESET => Some(crate::cli::commands::reset::spec()),
		cmd::SCALE => Some(crate::cli::commands::scale::spec()),
		cmd::STARTUP => Some(crate::cli::commands::startup::spec()),
		cmd::UPDATE => Some(crate::cli::commands::update::spec()),
		cmd::VERSION => Some(crate::cli::commands::version::spec()),
		_ => None,
	}
}

/// Names recognised by the stubbed `run_command` dispatch. Phase 6c
/// commands drop out so [`run_command`] in `mod.rs` can forward them
/// through [`run_dispatch`]. Mirrors [`register_all`].
#[must_use]
pub fn is_stubbed(name: &str) -> bool {
	if PORTED.contains(&name) {
		return false;
	}
	matches!(
		name,
		cmd::LIST
			| cmd::START
			| cmd::STOP
			| cmd::RESTART
			| cmd::DELETE
			| cmd::STARTUP
			| cmd::VERSION
			| cmd::APPLY
			| cmd::EXPORT
			| cmd::RELOAD
			| cmd::RESET
			| cmd::SCALE
			| cmd::FLUSH
			| cmd::UPDATE
			| cmd::INSTALL_TOOLS
			| cmd::EXEC_ENV
			| cmd::EXEC_SANDBOX
			| cmd::COMPLETION
	)
}

/// Names for which `print_command_help_to` should render the stub spec.
/// Same membership predicate as [`is_stubbed`] today; the two lists stay
/// in sync so a future mismatch is a deliberate decision, not drift.
#[must_use]
pub fn has_help(name: &str) -> bool {
	is_stubbed(name) && name != cmd::HELP
}

/// Dispatch a phase 6c command to its real entry point. The
/// dispatcher calls this before falling back to the stub path; a
/// `None` return tells the dispatcher this name is still stubbed.
/// Errors produced by the real command are returned boxed so the
/// dispatcher can render them via the shared error machinery.
pub fn run_dispatch(name: &str, args: &[String]) -> Option<Box<dyn std::error::Error>> {
	match name {
		cmd::LOGS => from_logs(args),
		cmd::SHOW => from_show(args),
		cmd::MONIT => from_monit(args),
		_ => None,
	}
}

fn from_logs(args: &[String]) -> Option<Box<dyn std::error::Error>> {
	logs::run(args)
		.err()
		.map(|e| Box::<dyn std::error::Error>::from(format!("{e}")))
}

fn from_show(args: &[String]) -> Option<Box<dyn std::error::Error>> {
	// `show::run` constructs its own transport client when the
	// dispatcher passes `None`, mirroring Go's `show.Run(nil, args)`.
	show::run(None, args)
		.err()
		.map(|e| Box::<dyn std::error::Error>::from(format!("{e}")))
}

fn from_monit(args: &[String]) -> Option<Box<dyn std::error::Error>> {
	// `monit::run` takes a borrowed client and an events iterator;
	// passing `None` lets the module build its own transport client,
	// and the stdin events iterator drives the render loop without
	// touching the terminal directly.
	monit::run(None, args, &mut monit::stdin_events())
		.err()
		.map(|e| Box::<dyn std::error::Error>::from(format!("{e}")))
}
