//! Dial a Unix socket with a timeout. Provided for parity with the Go
//! `dial(path, timeout)` helper; the [`crate::ipc::transport::Client`] type
//! uses [`std::os::unix::net::UnixStream::connect`] directly so this is
//! here for tests that want to exercise the path-resolution surface
//! independently.

use std::os::unix::net::UnixStream;
use std::path::Path;
use std::time::Duration;

use crate::ipc::transport::TransportError;

/// Connect to `path`, applying `timeout` to the dial itself.
#[allow(dead_code)]
pub fn dial(path: impl AsRef<Path>, timeout: Duration) -> Result<UnixStream, TransportError> {
	let path = path.as_ref();
	let stream = UnixStream::connect(path)
		.map_err(|e| TransportError::Dial(path.display().to_string(), e))?;
	let _ = stream.set_read_timeout(Some(timeout));
	let _ = stream.set_write_timeout(Some(timeout));
	Ok(stream)
}
