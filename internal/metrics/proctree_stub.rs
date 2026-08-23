//! Non-Linux stub for [`get_process_tree`].
//!
//! Mirrors the Go `proctree_stub.go`: the function exists on every
//! platform so callers do not need `#[cfg]` of their own, but on anything
//! other than Linux it returns a fixed error. The crate must compile here
//! even though `/proc` does not.

use super::{ChildStat, MetricsError};

/// Always errors on non-Linux platforms. The daemon's `tree` command is
/// therefore unavailable there; the process can still start and report
/// other state.
pub fn get_process_tree(_root_pid: i32) -> Result<Vec<ChildStat>, MetricsError> {
	Err(MetricsError::Io(std::io::Error::new(
		std::io::ErrorKind::Unsupported,
		"process tree not supported on this platform",
	)))
}
