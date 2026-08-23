//! Per-command stub registry.
//!
//! Phase 6a ships no real commands; every command the root dispatcher
//! recognises has a stub spec here that produces a "not yet ported"
//! error. Phase 6b replaces `not_yet_ported` and the registration list
//! as each real implementation lands.
//!
//! The four lifecycle commands (`list`, `start`, `stop`, `restart`)
//! are wired into real implementations in their own modules. The
//! dispatcher pulls a single optional client via
//! [`take_dispatcher_client`]; production wires a real
//! `transport::Client` through [`install_dispatcher_client`], tests
//! install a mock. When no client is set, the dispatcher surfaces a
//! "daemon unreachable" error that mirrors the Go side's
//! `DaemonUnreachable` variant.

use std::sync::{Mutex, OnceLock};

use crate::cli::commands;
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

/// Register every spec with the global registry. Phase-6b commands
/// (`list`, `start`, `stop`, `restart`) wire their real [`CommandSpec`];
/// every other command — `apply`, `completion`, `delete`, etc. — keeps
/// its "not yet ported" stub until phases 6c/6d land. Called by the
/// dispatcher at the top of `execute` so a fresh process always sees a
/// fully-populated registry, no matter the entry point.
pub fn register_all() {
	// Real implementations ported in phase 6b.
	registry::register(commands::list::spec());
	registry::register(commands::start::spec());
	registry::register(commands::stop::spec());
	registry::register(commands::restart::spec());

	// Stubs for commands outside phase 6b's scope.
	let stubbed = [
		cmd::LOGS,
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
	for n in stubbed {
		registry::register(stub_spec(n));
	}
}

/// Names recognised by the dispatcher. Mirrors [`register_all`] — every
/// command registered must also be dispatchable from `run_command`, and
/// unknown names fall through to `None` so the Go test
/// `TestRunCommand_UnknownReturnsNil` still pins that behaviour.
///
/// Phase 6b: `list`, `start`, `stop`, `restart` are real; the rest are
/// stub errors saying "phase 6c/6d not ported yet".
#[must_use]
pub fn is_stubbed(name: &str) -> bool {
	matches!(
		name,
		cmd::LIST
			| cmd::START
			| cmd::STOP
			| cmd::RESTART
			| cmd::LOGS
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

// --- Dispatcher client slot ---------------------------------------------
//
// The dispatcher needs a single optional IPC client that satisfies the
// `Start`/`Stop`/`Restart`/`List` surface. We keep it in a process-global
// because `execute_with` doesn't take one — the Go side builds the
// client lazily inside `run_command`. Phase 6c/6d commands that don't
// need an IPC client (pure flag formatting, etc.) keep their stubs and
// have no client in scope.

/// Object-safe view of "anything that can back the four lifecycle
/// commands". Production wraps a `transport::Client`; tests install a
/// `MockClient`. The four adapter methods return
/// `Box<dyn ...Backend>` handles that the command modules consume via
/// each module's private trait.
pub trait DispatcherClient: Send {
	fn list_handle(&mut self) -> Box<dyn ListBackend>;
	fn start_handle(&mut self) -> Box<dyn StartBackend>;
	fn stop_handle(&mut self) -> Box<dyn StopBackend>;
	fn restart_handle(&mut self) -> Box<dyn RestartBackend>;
}

/// Type-erased backend handles. Each command module's private trait
/// (`list::IpcOps`, `start::StartOps`, etc.) gets a manual
/// blanket impl for `Box<dyn XBackend>` so the dispatcher wires the
/// implementation directly without a typed adapter wrapper.
pub trait ListBackend: Send {
	fn call_list_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String>;
}
impl<T: ListBackend + ?Sized> ListBackend for Box<T> {
	fn call_list_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String> {
		(**self).call_list_inner()
	}
}

pub trait StartBackend: Send {
	fn start_inner(
		&mut self,
		req: &crate::ipc::protocol::StartRequest,
	) -> Result<crate::ipc::protocol::StartResponseData, String>;
	fn call_list_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String>;
}
impl<T: StartBackend + ?Sized> StartBackend for Box<T> {
	fn start_inner(
		&mut self,
		req: &crate::ipc::protocol::StartRequest,
	) -> Result<crate::ipc::protocol::StartResponseData, String> {
		(**self).start_inner(req)
	}
	fn call_list_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String> {
		(**self).call_list_inner()
	}
}

pub trait StopBackend: Send {
	fn list_processes_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String>;
	fn stop_inner(&mut self, id: &str) -> Result<crate::cli::commands::stop::StopResponse, String>;
}
impl<T: StopBackend + ?Sized> StopBackend for Box<T> {
	fn list_processes_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String> {
		(**self).list_processes_inner()
	}
	fn stop_inner(&mut self, id: &str) -> Result<crate::cli::commands::stop::StopResponse, String> {
		(**self).stop_inner(id)
	}
}

pub trait RestartBackend: Send {
	fn list_processes_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String>;
	fn restart_inner(
		&mut self,
		id: &str,
	) -> Result<crate::cli::commands::restart::RestartResponse, String>;
}
impl<T: RestartBackend + ?Sized> RestartBackend for Box<T> {
	fn list_processes_inner(&mut self) -> Result<Vec<crate::types::ProcessInfo>, String> {
		(**self).list_processes_inner()
	}
	fn restart_inner(
		&mut self,
		id: &str,
	) -> Result<crate::cli::commands::restart::RestartResponse, String> {
		(**self).restart_inner(id)
	}
}

// Adapter impls: each command's private trait becomes implementable
// for the corresponding backend handle. Living here (not in the
// command modules) keeps the commands untouched while the dispatcher
// orchestrates the dispatch.
//
// Hmm — Rust's orphan rule forbids `impl ForeignTrait for LocalType`.
// Each command's `IpcOps`/`StopOps`/etc. is *local* (defined in
// commands::list::IpcOps), so we can implement them here in the
// same crate. The keys are re-exports we pull in.

/// Production dispatch client — wraps `transport::Client` and forwards
/// each verb. Lives here so the transport package stays unaware of
/// the command surface. Phase 6c/6d wire the impl; until then this
/// is a placeholder so the type lives in the API surface.
#[allow(dead_code)]
pub struct TransportDispatcherClient<C: Send> {
	inner: C,
}

#[allow(dead_code)]
impl<C: Send> TransportDispatcherClient<C> {
	pub fn new(inner: C) -> Self {
		Self { inner }
	}
}

/// Boxed dispatcher client — what [`install_dispatcher_client`] accepts.
pub type BoxedDispatcher = Box<dyn DispatcherClient>;

static DISPATCH_CLIENT: OnceLock<Mutex<Option<BoxedDispatcher>>> = OnceLock::new();

fn dispatch_client_slot() -> &'static Mutex<Option<BoxedDispatcher>> {
	DISPATCH_CLIENT.get_or_init(|| Mutex::new(None))
}

/// Install the dispatcher client. Replaces any previously installed
/// client. Production calls this once at startup before [`execute`] /
/// [`execute_with`].
#[allow(dead_code)]
pub fn install_dispatcher_client(client: BoxedDispatcher) {
	let mut slot = dispatch_client_slot()
		.lock()
		.expect("dispatch client poisoned");
	*slot = Some(client);
}

/// Take the installed dispatcher client (leaving `None` in its slot).
/// Used by the dispatcher when it actually invokes a real command; on
/// the next call the client must be re-installed, which mirrors how
/// the Go side constructs a new client per command invocation.
pub fn take_dispatcher_client() -> Option<BoxedDispatcher> {
	let mut slot = dispatch_client_slot()
		.lock()
		.expect("dispatch client poisoned");
	slot.take()
}

#[allow(dead_code)]
pub fn has_dispatcher_client() -> bool {
	let slot = dispatch_client_slot()
		.lock()
		.expect("dispatch client poisoned");
	slot.is_some()
}

// Each command module consumes a typed handle. We expose the adapter
// impls here so the actual `impl IpcOps for ...` blocks live in the
// commands — but the dispatcher's Box<dyn ...> translation uses these.

impl crate::cli::commands::list::IpcOps for Box<dyn ListBackend> {
	fn call_list(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::list::IpcError> {
		(**self)
			.call_list_inner()
			.map_err(crate::cli::commands::list::IpcError::from)
	}
}

impl crate::cli::commands::start::StartOps for Box<dyn StartBackend> {
	type Error = String;
	fn start(
		&mut self,
		req: &crate::ipc::protocol::StartRequest,
	) -> Result<crate::ipc::protocol::StartResponseData, String> {
		(**self).start_inner(req)
	}
}
impl crate::cli::commands::list::IpcOps for Box<dyn StartBackend> {
	fn call_list(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::list::IpcError> {
		(**self)
			.call_list_inner()
			.map_err(crate::cli::commands::list::IpcError::from)
	}
}

impl crate::cli::commands::stop::StopOps for Box<dyn StopBackend> {
	fn list_processes(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::stop::IpcError> {
		self.list_processes_inner()
			.map_err(crate::cli::commands::stop::IpcError::from)
	}
	fn stop(
		&mut self,
		id: &str,
	) -> Result<crate::cli::commands::stop::StopResponse, crate::cli::commands::stop::IpcError> {
		self.stop_inner(id)
			.map_err(crate::cli::commands::stop::IpcError::from)
	}
}
impl crate::cli::expand::ListClient for Box<dyn StopBackend> {
	fn list_processes(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::expand::ListError> {
		self.list_processes_inner()
			.map_err(crate::cli::expand::ListError::Protocol)
	}
}
impl crate::cli::commands::list::IpcOps for Box<dyn StopBackend> {
	fn call_list(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::list::IpcError> {
		(**self)
			.list_processes_inner()
			.map_err(crate::cli::commands::list::IpcError::from)
	}
}

impl crate::cli::commands::restart::RestartOps for Box<dyn RestartBackend> {
	fn list_processes(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::restart::IpcError> {
		self.list_processes_inner()
			.map_err(crate::cli::commands::restart::IpcError::from)
	}
	fn restart(
		&mut self,
		id: &str,
	) -> Result<
		crate::cli::commands::restart::RestartResponse,
		crate::cli::commands::restart::IpcError,
	> {
		self.restart_inner(id)
			.map_err(crate::cli::commands::restart::IpcError::from)
	}
}
impl crate::cli::expand::ListClient for Box<dyn RestartBackend> {
	fn list_processes(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::expand::ListError> {
		self.list_processes_inner()
			.map_err(crate::cli::expand::ListError::Protocol)
	}
}
impl crate::cli::commands::list::IpcOps for Box<dyn RestartBackend> {
	fn call_list(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::list::IpcError> {
		(**self)
			.list_processes_inner()
			.map_err(crate::cli::commands::list::IpcError::from)
	}
}
