//! Collector factory.
//!
//! Tries the per-process tree collector first (accurate memory via
//! `/proc/<pid>/stat`), then falls back to the cgroup v2 collector (only
//! accurate when the PID runs in a dedicated cgroup). The Go equivalent
//! uses the same precedence.
//!
//! Linux-only. A non-Linux factory stub will land if the platform ever
//! needs one — for now `get_process_tree` is the only cross-platform
//! surface and its stub lives in `proctree_stub.rs`.

use super::cgroup::new_cgroup_collector;
use super::proctree::new_proc_tree_collector;
use super::{Collector, MetricsError};

/// Best-effort factory: prefer the process tree collector, fall back to
/// cgroup v2.
pub fn new_collector(pid: i32) -> Result<Box<dyn Collector>, MetricsError> {
	if let Ok(c) = new_proc_tree_collector(pid) {
		return Ok(Box::new(c));
	}
	Ok(Box::new(new_cgroup_collector(pid)?))
}
