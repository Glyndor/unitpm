//! Client side of the IPC transport.
//!
//! `IPCClient` is the trait call sites depend on. `Client` is the concrete
//! implementation that dials the Unix socket and exchanges newline-delimited
//! JSON envelopes.

use std::io::{BufRead, BufReader, Write};
use std::net::Shutdown;
use std::os::unix::net::UnixStream;
use std::path::Path;

use serde::de::DeserializeOwned;
use serde::Serialize;

use crate::ipc::protocol::{self, RawMessage, RemoteError};
use crate::ipc::transport::limits::{ReadTimeout, WriteTimeout};
use crate::ipc::transport::socket_unix::{get_socket_path, SocketPathError};
use crate::jsonx;
use crate::spec;

/// Trait the daemon and CLI depend on for sending IPC commands.
pub trait IPCClient {
	/// Send `command` with `params` and decode the response into `result`.
	fn call<T, R>(
		&mut self,
		command: &str,
		params: Option<&T>,
		result: Option<&mut R>,
	) -> Result<(), TransportError>
	where
		T: Serialize,
		R: DeserializeOwned;

	/// Close the underlying connection.
	fn close(&mut self) -> Result<(), TransportError>;
}

/// Concrete client backed by a [`UnixStream`].
pub struct Client {
	stream: UnixStream,
}

impl Client {
	/// Open a connection to the socket returned by [`get_socket_path`].
	pub fn new() -> Result<Self, TransportError> {
		let path = get_socket_path()?;
		let stream = UnixStream::connect(&path)
			.map_err(|e| TransportError::daemon_unreachable(path.clone(), e))?;
		Ok(Self { stream })
	}

	/// Build a client against an explicit socket path. Tests use this to
	/// avoid touching `UNITPM_SOCKET` / `XDG_RUNTIME_DIR`.
	pub fn connect_to(path: impl AsRef<Path>) -> Result<Self, TransportError> {
		let path = path.as_ref().display().to_string();
		let stream = UnixStream::connect(&path)
			.map_err(|e| TransportError::daemon_unreachable(path.clone(), e))?;
		Ok(Self { stream })
	}
}

impl IPCClient for Client {
	fn call<T, R>(
		&mut self,
		command: &str,
		params: Option<&T>,
		result: Option<&mut R>,
	) -> Result<(), TransportError>
	where
		T: Serialize,
		R: DeserializeOwned,
	{
		let req_id = spec::generate_id();

		let param_bytes = match params {
			Some(p) => Some(jsonx::marshal(p).map_err(TransportError::from)?),
			None => None,
		};

		let req = protocol::Request {
			version: protocol::VERSION,
			id: req_id.clone(),
			command: command.to_string(),
			params: param_bytes.as_ref().map(|b| RawMessage::from_bytes(b)),
		};

		self.stream
			.set_write_timeout(Some(WriteTimeout))
			.map_err(TransportError::Io)?;
		let bytes = jsonx::marshal(&req).map_err(TransportError::from)?;
		self.stream.write_all(&bytes).map_err(TransportError::Io)?;
		self.stream.write_all(b"\n").map_err(TransportError::Io)?;
		self.stream.flush().map_err(TransportError::Io)?;

		self.stream
			.set_read_timeout(Some(ReadTimeout))
			.map_err(TransportError::Io)?;

		let cloned = self.stream.try_clone().map_err(TransportError::Io)?;
		let mut reader = BufReader::new(cloned);
		let mut line = String::new();
		let n = reader.read_line(&mut line).map_err(TransportError::Io)?;
		if n == 0 {
			return Err(TransportError::Io(std::io::Error::new(
				std::io::ErrorKind::UnexpectedEof,
				"connection closed by server",
			)));
		}

		let resp: protocol::Response =
			jsonx::unmarshal(line.as_bytes()).map_err(TransportError::from)?;
		if resp.id != req_id {
			return Err(TransportError::IdMismatch {
				got: resp.id,
				want: req_id,
			});
		}

		if resp.status == protocol::STATUS_ERROR {
			let err = resp.error.unwrap_or(Box::new(protocol::Error {
				code: "UNKNOWN".into(),
				message: "unknown ipc error".into(),
				data: None,
			}));
			return Err(TransportError::Remote(RemoteError {
				code: err.code,
				message: err.message,
				data: err.data,
			}));
		}

		if let (Some(_target), Some(res_bytes)) = (result, resp.result.as_ref()) {
			let target = _target;
			*target = jsonx::unmarshal(res_bytes.as_bytes()).map_err(TransportError::from)?;
		}

		Ok(())
	}

	fn close(&mut self) -> Result<(), TransportError> {
		let _ = self.stream.shutdown(Shutdown::Both);
		Ok(())
	}
}

/// Errors surfaced by the transport layer.
#[derive(Debug)]
pub enum TransportError {
	Io(std::io::Error),
	Json(jsonx::Error),
	IdMismatch {
		got: String,
		want: String,
	},
	Remote(RemoteError),
	SocketPath(SocketPathError),
	Dial(String, std::io::Error),
	DaemonUnreachable {
		path: String,
		hint: String,
		original: String,
	},
}

impl TransportError {
	pub(crate) fn daemon_unreachable(path: String, original: std::io::Error) -> Self {
		let msg = original.to_string();
		let user_mode = path.contains("/run/user/")
			|| path.starts_with(std::env::var("XDG_RUNTIME_DIR").as_deref().unwrap_or(""));
		let hint = if user_mode {
			"start the daemon in the background:\n    unitpmd &\n  or enable user-mode startup:\n    unitpmctl startup"
		} else {
			"start the system daemon:\n    sudo systemctl start unitpmd\n  if you just installed, also run:\n    sudo unitpmctl startup"
		};
		TransportError::DaemonUnreachable {
			path,
			hint: hint.to_string(),
			original: msg,
		}
	}
}

impl std::fmt::Display for TransportError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			TransportError::Io(e) => write!(f, "io error: {e}"),
			TransportError::Json(e) => write!(f, "json error: {e}"),
			TransportError::IdMismatch { got, want } => {
				write!(f, "response ID mismatch: got {got}, want {want}")
			}
			TransportError::Remote(e) => write!(f, "{e}"),
			TransportError::SocketPath(e) => write!(f, "{e}"),
			TransportError::Dial(path, e) => write!(f, "dial {path}: {e}"),
			TransportError::DaemonUnreachable {
				path,
				hint,
				original,
			} => write!(
				f,
				"cannot reach the unitpm daemon at {path}\n  {hint}\n\n  original error: {original}"
			),
		}
	}
}

impl std::error::Error for TransportError {}

impl From<std::io::Error> for TransportError {
	fn from(e: std::io::Error) -> Self {
		TransportError::Io(e)
	}
}

impl From<jsonx::Error> for TransportError {
	fn from(e: jsonx::Error) -> Self {
		TransportError::Json(e)
	}
}

impl From<SocketPathError> for TransportError {
	fn from(e: SocketPathError) -> Self {
		TransportError::SocketPath(e)
	}
}
