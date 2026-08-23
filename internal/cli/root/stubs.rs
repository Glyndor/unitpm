//! Per-command stub registry.
//!
//! Phase 6a ships no real commands; every command the root dispatcher
//! recognises has a stub spec here that produces a "not yet ported"
//! error. Phase 6b replaces `not_yet_ported` and the registration list
//! as each real implementation lands.

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

/// Register every stub spec with the global registry. Called by the
/// dispatcher at the top of `execute` so a fresh process always sees a
/// fully-populated registry, no matter the entry point.
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
		registry::register(stub_spec(n));
	}
}

/// Names recognised by the stubbed `run_command` dispatch. The list
/// mirrors [`register_all`] — every command registered must also produce a
/// stub error here, and unknown names fall through to `None` so the
/// Go test `TestRunCommand_UnknownReturnsNil` still pins that behaviour.
#[must_use]
pub fn is_stubbed(name: &str) -> bool {
	matches!(
		name,
		cmd::LIST
			| cmd::LOGS
			| cmd::START
			| cmd::STOP
			| cmd::RESTART
			| cmd::DELETE
			| cmd::STARTUP
			| cmd::VERSION
			| cmd::APPLY
			| cmd::EXPORT
			| cmd::SHOW
			| cmd::MONIT
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
