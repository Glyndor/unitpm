//! Per-connection loop and bounded line reader for the IPC server.
//!
//! The bounded reader enforces [`crate::ipc::transport::limits::MaxMsgSize`]:
//! a peer sending more is rejected with `ERR_LIMITS` and the connection
//! is closed.

use std::io::{BufRead, BufReader, Write};

#[allow(unused_imports)]
use crate::ipc::protocol::Response;
use crate::ipc::transport::identity_unix::validate_identity;
use crate::ipc::transport::limits::{MaxMsgSize, ReadTimeout, WriteTimeout};
use crate::ipc::transport::ratelimit::RateLimiter;
use crate::ipc::transport::server::{HandlersSnapshot, UniversalRequest};
use crate::ipc::transport::server_dispatch::{dispatch, dispatch_start, send_error};
use crate::jsonx;

/// Drive one connection from accept to close. The peer is authenticated,
/// then the per-request loop runs until the peer hangs up, the read
/// deadline fires, or the connection's handler returns.
pub(crate) fn handle_connection(
	stream: std::os::unix::net::UnixStream,
	handlers: std::sync::Arc<HandlersSnapshot>,
	rate_limit: std::sync::Arc<RateLimiter>,
) {
	let identity = match validate_identity(&stream) {
		Ok(id) => id,
		Err(e) => {
			eprintln!("IPC connection rejected: validateIdentity failed: {e}");
			return;
		}
	};

	let cloned = match stream.try_clone() {
		Ok(c) => c,
		Err(_) => return,
	};
	let mut reader = BoundedLineReader::new(BufReader::new(cloned));
	let mut writer = stream;

	loop {
		if writer.set_read_timeout(Some(ReadTimeout)).is_err() {
			return;
		}

		let line = match reader.read_line() {
			Ok(Some(line)) => line,
			Ok(None) => return,
			Err(ReadError::TooLong) => {
				send_error(&mut writer, "", "ERR_LIMITS", "Message too large");
				return;
			}
			Err(ReadError::Deadline) => {
				send_error(&mut writer, "", "ERR_TIMEOUT", "Read timed out");
				return;
			}
			Err(ReadError::Io(_)) => return,
		};

		let univ: UniversalRequest = match jsonx::unmarshal(&line) {
			Ok(v) => v,
			Err(_) => {
				send_error(&mut writer, "", "ERR_BAD_REQUEST", "Invalid JSON");
				return;
			}
		};

		if let Ok(uid) = identity.uid.parse::<u32>() {
			if !rate_limit.allow(uid) {
				send_error(
					&mut writer,
					&univ.id,
					"ERR_RATE_LIMIT",
					"IPC rate limit exceeded for this UID",
				);
				continue;
			}
		}

		let bytes = if univ.kind == "start" {
			let resp = dispatch_start(&univ, &handlers, &identity);
			match jsonx::marshal(&resp) {
				Ok(b) => b,
				Err(_) => return,
			}
		} else {
			let resp = dispatch(&univ, &handlers, &identity);
			match jsonx::marshal(&resp) {
				Ok(b) => b,
				Err(_) => return,
			}
		};

		if writer.set_write_timeout(Some(WriteTimeout)).is_err() {
			return;
		}
		if writer.write_all(&bytes).is_err() {
			return;
		}
		if writer.write_all(b"\n").is_err() {
			return;
		}
		if writer.flush().is_err() {
			return;
		}
	}
}

/// Single-message reader that enforces [`MaxMsgSize`].
pub(crate) struct BoundedLineReader<R: BufRead> {
	inner: R,
}

#[derive(Debug)]
#[allow(dead_code)]
pub(crate) enum ReadError {
	Io(std::io::Error),
	TooLong,
	Deadline,
}

impl<R: BufRead> BoundedLineReader<R> {
	pub(crate) fn new(inner: R) -> Self {
		Self { inner }
	}

	pub(crate) fn read_line(&mut self) -> Result<Option<Vec<u8>>, ReadError> {
		let mut buf = Vec::with_capacity(4096);
		loop {
			let available = self.inner.fill_buf().map_err(ReadError::Io)?;
			if available.is_empty() {
				return Ok(if buf.is_empty() { None } else { Some(buf) });
			}
			match available.iter().position(|&b| b == b'\n') {
				Some(idx) => {
					let take = idx + 1;
					if buf.len() + take > MaxMsgSize + 1 {
						return Err(ReadError::TooLong);
					}
					buf.extend_from_slice(&available[..take]);
					self.inner.consume(take);
					return Ok(Some(buf));
				}
				None => {
					if buf.len() + available.len() > MaxMsgSize + 1 {
						return Err(ReadError::TooLong);
					}
					buf.extend_from_slice(available);
					let n = available.len();
					self.inner.consume(n);
				}
			}
		}
	}
}

// Avoid unused warnings for items consumed via the crate's public path.
#[allow(dead_code)]
fn _assert_path() {}
