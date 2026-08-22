//! IPC server.
//!
//! Accepts connections on the Unix socket, authenticates the peer via
//! `SO_PEERCRED`, rate-limits per UID, and dispatches each request to the
//! registered handler. The server enforces three concurrency ceilings:
//!
//! - [`MaxConnections`] (semaphore) bounds concurrent handlers.
//! - [`MaxMsgSize`] (bounded line reader) caps a single message.
//! - [`RateLimiter`] caps per-UID throughput.
//!
//! Handlers are called inside a thread per connection. A panic in the
//! handler is caught and the connection closed; the server keeps running.

use std::collections::HashMap;
use std::os::unix::net::UnixListener;
use std::panic::{catch_unwind, AssertUnwindSafe};
use std::path::PathBuf;
use std::sync::{Arc, Mutex, RwLock};
use std::thread;

use crate::ipc::protocol::RawMessage;
use crate::ipc::transport::limits::{MaxConnections as MAX_CONN, WriteTimeout};
use crate::ipc::transport::listener_unix::listen;
use crate::ipc::transport::ratelimit::RateLimiter;
use crate::ipc::transport::socket_unix::get_socket_path;
use serde::{Deserialize, Serialize};

pub(crate) use crate::ipc::transport::server_loop::handle_connection;

/// Handler invoked by the server for a registered command. Receives the raw
/// JSON parameters and returns either a result or an error.
pub type CommandHandler =
	Box<dyn Fn(RequestContext, RawMessage) -> Result<RawMessage, String> + Send + Sync>;

/// Per-request context handed to a handler. Carries the peer [`Identity`].
#[derive(Debug, Clone)]
pub struct RequestContext {
	pub identity: crate::ipc::transport::Identity,
}

/// Union request type — accepts either the standard envelope or a `start`
/// payload. The discriminator is `type`: `start` or anything else.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct UniversalRequest {
	/// `start` for the start payload, anything else (or empty) for the
	/// standard envelope.
	#[serde(rename = "type", default)]
	pub kind: String,
	#[serde(default)]
	pub version: i32,
	#[serde(default)]
	pub id: String,
	#[serde(default)]
	pub command: String,
	#[serde(default)]
	pub params: Option<RawMessage>,
	#[serde(default, rename = "protocol_version")]
	pub protocol_version: i32,
	#[serde(default, rename = "request_id")]
	pub request_id: String,
	#[serde(default)]
	pub spec: Option<RawMessage>,
}

/// Server-side error.
#[derive(Debug)]
pub enum ServerError {
	NoSocketPath(String),
	Listen(String),
	AlreadyStarted,
}

impl std::fmt::Display for ServerError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			ServerError::NoSocketPath(e) => write!(f, "no socket path: {e}"),
			ServerError::Listen(e) => write!(f, "listen: {e}"),
			ServerError::AlreadyStarted => f.write_str("server already started"),
		}
	}
}

impl std::error::Error for ServerError {}

/// The daemon-side IPC server.
pub struct Server {
	handlers: RwLock<HashMap<String, Arc<CommandHandler>>>,
	listener: Mutex<Option<Arc<UnixListener>>>,
	rate_limit: Arc<RateLimiter>,
	conn_cap: Arc<ConnCap>,
}

impl Server {
	/// Build a server with default rate-limit configuration.
	#[must_use]
	pub fn new() -> Self {
		Self::with_rate_limit(Arc::new(RateLimiter::with_capacity_and_refill(
			200.0, 100.0,
		)))
	}

	/// Build a server with an explicit rate limiter (used in tests so the
	/// burst/refill can be pinned).
	#[must_use]
	pub fn with_rate_limit(rate_limit: Arc<RateLimiter>) -> Self {
		Self {
			handlers: RwLock::new(HashMap::new()),
			listener: Mutex::new(None),
			rate_limit,
			conn_cap: Arc::new(ConnCap::new(MAX_CONN)),
		}
	}

	/// Register a handler for `command`.
	pub fn register<H>(&self, command: &str, handler: H)
	where
		H: Fn(RequestContext, RawMessage) -> Result<RawMessage, String> + Send + Sync + 'static,
	{
		let mut g = self.handlers.write().unwrap_or_else(|e| e.into_inner());
		g.insert(command.to_string(), Arc::new(Box::new(handler)));
	}

	/// `true` when a handler is registered for `command`.
	#[must_use]
	pub fn has_handler(&self, command: &str) -> bool {
		let g = self.handlers.read().unwrap_or_else(|e| e.into_inner());
		g.contains_key(command)
	}

	/// Start the accept loop. Returns once the listener is bound; spawns a
	/// background thread to keep accepting connections.
	pub fn start(&self) -> Result<PathBuf, ServerError> {
		let path = get_socket_path().map_err(|e| ServerError::NoSocketPath(e.to_string()))?;
		let listener = listen(&path).map_err(|e| ServerError::Listen(e.to_string()))?;
		let listener = Arc::new(listener);
		{
			let mut g = self.listener.lock().unwrap_or_else(|e| e.into_inner());
			if g.is_some() {
				return Err(ServerError::AlreadyStarted);
			}
			*g = Some(listener.clone());
		}

		let handlers = Arc::new(HandlersSnapshot(
			self.handlers
				.read()
				.unwrap_or_else(|e| e.into_inner())
				.clone(),
		));
		let rate_limit = self.rate_limit.clone();
		let conn_cap = self.conn_cap.clone();

		thread::spawn(move || loop {
			let (stream, _) = match listener.accept() {
				Ok(s) => s,
				Err(_) => return,
			};
			let permit = match conn_cap.try_acquire() {
				Some(p) => p,
				None => {
					let _ = stream.shutdown(std::net::Shutdown::Both);
					continue;
				}
			};
			let handlers = handlers.clone();
			let rate_limit = rate_limit.clone();
			thread::spawn(move || {
				let result = catch_unwind(AssertUnwindSafe(|| {
					handle_connection(stream, handlers, rate_limit)
				}));
				if let Err(e) = result {
					eprintln!("panic in IPC connection handler: {e:?}");
				}
				drop(permit);
			});
		});

		Ok(PathBuf::from(path))
	}

	/// Close the listener.
	pub fn close(&self) {
		let listener = {
			let mut g = self.listener.lock().unwrap_or_else(|e| e.into_inner());
			g.take()
		};
		if let Some(l) = listener {
			let _ = l.set_nonblocking(true);
			while let Ok((s, _)) = l.accept() {
				let _ = s.shutdown(std::net::Shutdown::Both);
			}
		}
	}

	#[allow(dead_code)]
	#[cfg(test)]
	pub fn conn_cap_remaining_for_test(&self) -> usize {
		self.conn_cap.remaining()
	}

	#[allow(dead_code)]
	fn handlers_snapshot(&self) -> Arc<HandlersSnapshot> {
		Arc::new(HandlersSnapshot(
			self.handlers
				.read()
				.unwrap_or_else(|e| e.into_inner())
				.clone(),
		))
	}
}

impl Default for Server {
	fn default() -> Self {
		Self::new()
	}
}

/// Snapshot of the handler registry passed to a connection thread.
pub(crate) struct HandlersSnapshot(pub(crate) HashMap<String, Arc<CommandHandler>>);

/// Counting semaphore for concurrent connections.
pub(crate) struct ConnCap {
	state: Mutex<CapState>,
}

pub(crate) struct CapState {
	count: usize,
}

impl ConnCap {
	pub(crate) fn new(capacity: usize) -> Self {
		Self {
			state: Mutex::new(CapState { count: capacity }),
		}
	}
	#[cfg(test)]
	fn remaining(&self) -> usize {
		self.state.lock().unwrap_or_else(|e| e.into_inner()).count
	}
	pub(crate) fn try_acquire(self: &Arc<Self>) -> Option<ConnPermit> {
		let mut g = self.state.lock().unwrap_or_else(|e| e.into_inner());
		if g.count == 0 {
			None
		} else {
			g.count -= 1;
			Some(ConnPermit { cap: self.clone() })
		}
	}
}

pub(crate) struct ConnPermit {
	cap: Arc<ConnCap>,
}

impl Drop for ConnPermit {
	fn drop(&mut self) {
		let mut g = self.cap.state.lock().unwrap_or_else(|e| e.into_inner());
		g.count += 1;
	}
}

// Keep the imports honest — `WriteTimeout` is referenced by the
// per-connection loop in the sibling module but the compiler still flags
// unused items otherwise.
#[allow(dead_code)]
fn _unused_path_check() {
	let _ = WriteTimeout;
}
