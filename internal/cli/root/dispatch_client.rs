//! The dispatcher's IPC client slot.
//!
//! Split out of `stubs.rs`, which was doing two unrelated jobs in one file:
//! deciding which commands are real, and holding the client the real ones
//! need. The org's limit is 500 lines and the file was 537.

use std::sync::{Arc, Mutex, MutexGuard, OnceLock};

use crate::ipc::protocol::{StartRequest, StartResponseData};
use crate::ipc::transport::{Client, IPCClient};
use crate::types::ProcessInfo;

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

/// Production dispatch client — wraps `transport::Client` behind an
/// `Arc<Mutex<..>>` so each `*_handle()` can hand out an owned backend
/// handle without cloning the transport stream. The transport package
/// stays unaware of the command surface: this is where the four
/// backend traits meet the real IPC client.
///
/// Construction is lazy — the entry point calls
/// [`TransportDispatcherClient::lazy`] and the first `call_*` opens
/// the socket, so `unitpm --help`, `unitpm version`, and an unknown
/// name never dial. This matches the Go behaviour where the four
/// lifecycle commands built their own clients only when invoked.
///
/// Construct via [`TransportDispatcherClient::lazy`] and hand it to
/// [`install_dispatcher_client`] before calling [`execute`]. The
/// dispatcher takes ownership of one `Box<dyn DispatcherClient>` per
/// process invocation.
pub struct TransportDispatcherClient {
	client: Arc<Mutex<Option<Client>>>,
}

impl TransportDispatcherClient {
	/// Build a client that dials the socket on first use. Errors
	/// surface from the first call that needs the socket, mirroring
	/// the Go `transport.NewClient()` flow that ran inside the
	/// lifecycle command modules when no client was supplied.
	#[must_use]
	pub fn lazy() -> Self {
		Self {
			client: Arc::new(Mutex::new(None)),
		}
	}

	/// Wrap an already-connected transport client. Tests use this to
	/// avoid the lazy-dial path.
	#[must_use]
	pub fn from_client(client: Client) -> Self {
		Self {
			client: Arc::new(Mutex::new(Some(client))),
		}
	}

	/// Lock the inner slot, dialing on first use. Returns a
	/// [`LockedClient`] that derefs to `Client`; every backend method
	/// uses this to make sure the socket is open before issuing a
	/// request.
	fn lock_or_dial(&self) -> Result<LockedClient<'_>, String> {
		let guard = self.client.lock().map_err(|e| e.to_string())?;
		Ok(LockedClient { guard })
	}
}

/// `MutexGuard` over `Option<Client>` that exposes `&mut Client`
/// after lazily dialing. The dial error short-circuits the deref so a
/// dead daemon surfaces through the backend traits' `Result<_, String>`
/// return type without `unwrap()`s in the call sites.
pub struct LockedClient<'a> {
	guard: MutexGuard<'a, Option<Client>>,
}

impl<'a> LockedClient<'a> {
	/// Borrow the underlying client, dialing the socket on first use.
	/// The dial result is cached so subsequent calls skip the connect.
	pub fn client(&mut self) -> Result<&mut Client, String> {
		if self.guard.is_none() {
			let client = Client::new().map_err(|e| e.to_string())?;
			*self.guard = Some(client);
		}
		Ok(self.guard.as_mut().expect("just initialised"))
	}
}

impl Clone for TransportDispatcherClient {
	fn clone(&self) -> Self {
		Self {
			client: Arc::clone(&self.client),
		}
	}
}

impl ListBackend for TransportDispatcherClient {
	fn call_list_inner(&mut self) -> Result<Vec<ProcessInfo>, String> {
		let mut guard = self.lock_or_dial()?;
		let mut procs: Vec<ProcessInfo> = Vec::new();
		guard
			.client()?
			.call::<(), Vec<ProcessInfo>>("list", None, Some(&mut procs))
			.map_err(|e| e.to_string())?;
		Ok(procs)
	}
}

impl StartBackend for TransportDispatcherClient {
	fn start_inner(&mut self, req: &StartRequest) -> Result<StartResponseData, String> {
		let mut guard = self.lock_or_dial()?;
		let mut data: StartResponseData = StartResponseData {
			id: String::new(),
			proc_id: None,
			pid: None,
			status: None,
			message: None,
			created_at: None,
		};
		guard
			.client()?
			.call::<StartRequest, StartResponseData>("start", Some(req), Some(&mut data))
			.map_err(|e| e.to_string())?;
		Ok(data)
	}

	fn call_list_inner(&mut self) -> Result<Vec<ProcessInfo>, String> {
		// Forward to the same `list` verb `StartBackend`'s command
		// path uses internally — the daemon's `start` verb also calls
		// the manager's process table, so this stays consistent.
		let mut guard = self.lock_or_dial()?;
		let mut procs: Vec<ProcessInfo> = Vec::new();
		guard
			.client()?
			.call::<(), Vec<ProcessInfo>>("list", None, Some(&mut procs))
			.map_err(|e| e.to_string())?;
		Ok(procs)
	}
}

impl StopBackend for TransportDispatcherClient {
	fn list_processes_inner(&mut self) -> Result<Vec<ProcessInfo>, String> {
		let mut guard = self.lock_or_dial()?;
		let mut procs: Vec<ProcessInfo> = Vec::new();
		guard
			.client()?
			.call::<(), Vec<ProcessInfo>>("list", None, Some(&mut procs))
			.map_err(|e| e.to_string())?;
		Ok(procs)
	}

	fn stop_inner(&mut self, id: &str) -> Result<crate::cli::commands::stop::StopResponse, String> {
		let mut guard = self.lock_or_dial()?;
		let mut resp: serde_json::Value = serde_json::Value::Null;
		let params = serde_json::json!({ "id": id });
		guard
			.client()?
			.call::<serde_json::Value, serde_json::Value>("stop", Some(&params), Some(&mut resp))
			.map_err(|e| e.to_string())?;
		Ok(crate::cli::commands::stop::StopResponse {
			status: resp
				.get("status")
				.and_then(|v| v.as_str())
				.unwrap_or("")
				.to_string(),
			id: resp
				.get("id")
				.and_then(|v| v.as_str())
				.unwrap_or("")
				.to_string(),
			was_running: resp
				.get("was_running")
				.and_then(|v| v.as_bool())
				.unwrap_or(false),
		})
	}
}

impl RestartBackend for TransportDispatcherClient {
	fn list_processes_inner(&mut self) -> Result<Vec<ProcessInfo>, String> {
		let mut guard = self.lock_or_dial()?;
		let mut procs: Vec<ProcessInfo> = Vec::new();
		guard
			.client()?
			.call::<(), Vec<ProcessInfo>>("list", None, Some(&mut procs))
			.map_err(|e| e.to_string())?;
		Ok(procs)
	}

	fn restart_inner(
		&mut self,
		id: &str,
	) -> Result<crate::cli::commands::restart::RestartResponse, String> {
		let mut guard = self.lock_or_dial()?;
		let mut resp: serde_json::Value = serde_json::Value::Null;
		let params = serde_json::json!({ "id": id });
		guard
			.client()?
			.call::<serde_json::Value, serde_json::Value>("restart", Some(&params), Some(&mut resp))
			.map_err(|e| e.to_string())?;
		Ok(crate::cli::commands::restart::RestartResponse {
			status: resp
				.get("status")
				.and_then(|v| v.as_str())
				.unwrap_or("")
				.to_string(),
			id: resp
				.get("id")
				.and_then(|v| v.as_str())
				.unwrap_or("")
				.to_string(),
		})
	}
}

impl DispatcherClient for TransportDispatcherClient {
	fn list_handle(&mut self) -> Box<dyn ListBackend> {
		Box::new(self.clone())
	}
	fn start_handle(&mut self) -> Box<dyn StartBackend> {
		Box::new(self.clone())
	}
	fn stop_handle(&mut self) -> Box<dyn StopBackend> {
		Box::new(self.clone())
	}
	fn restart_handle(&mut self) -> Box<dyn RestartBackend> {
		Box::new(self.clone())
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
