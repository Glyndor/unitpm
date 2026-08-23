//! Process resource usage metrics collection.
//!
//! Mirrors the Go `internal/metrics` package. The shared surface —
//! [`Metrics`], the [`Collector`] trait, and [`ChildStat`] — is always
//! compiled. Linux-only collectors live behind
//! `#[cfg(target_os = "linux")]` so the crate builds on every target even
//! where the kernel interfaces do not exist. The non-Linux fallback for
//! [`get_process_tree`] lives in `proctree_stub.rs`, mirroring the Go
//! `_linux` / `_stub` file split.
//!
//! The process tree snapshot is cached for one second so multiple collectors
//! running against the same /proc walk share one scan. The cache is
//! process-global; tests can clear it via
//! [`clear_proc_tree_cache_for_tests`], and a [`ProcTreeCacheGuard`] saves
//! and restores the cache value on `Drop` so a panicking test cannot leak
//! state into the next.

mod cgroup;
mod factory;
mod proctree;
#[cfg(not(target_os = "linux"))]
mod proctree_stub;

#[cfg(target_os = "linux")]
pub use cgroup::{new_cgroup_collector, CgroupCollector};
#[cfg(target_os = "linux")]
pub use factory::new_collector;
#[cfg(all(test, target_os = "linux"))]
pub use proctree::tests::{clear_proc_tree_cache_for_tests, ProcTreeCacheGuard};
#[cfg(target_os = "linux")]
pub use proctree::{get_ppid, get_process_tree, new_proc_tree_collector, ProcTreeCollector};

#[cfg(not(target_os = "linux"))]
pub use proctree_stub::get_process_tree;

use serde::{Deserialize, Serialize};
use std::time::SystemTime;

/// Process resource usage snapshot. Used internally between the collectors
/// and the manager; never serialised to JSON, so the timestamp can stay a
/// `SystemTime` without dragging in a date/time crate.
#[derive(Debug, Clone, PartialEq)]
pub struct Metrics {
	pub timestamp: SystemTime,
	pub memory_bytes: i64,
	pub cpu_percent: f64,
}

/// Source of [`Metrics`] samples. Each collector owns its own deltas so two
/// collectors can run side by side without interfering.
pub trait Collector {
	fn collect(&mut self) -> Result<Metrics, MetricsError>;

	/// Stable name of the concrete implementation, used by the factory
	/// test to verify precedence without resorting to a downcast.
	fn kind(&self) -> &'static str;
}

/// Concrete collector kinds. Mirrors the Go type-discriminating assertion
/// in `TestNewCollector_PrefersProcTree`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CollectorKind {
	ProcTree,
	Cgroup,
}

impl CollectorKind {
	#[must_use]
	pub const fn as_str(self) -> &'static str {
		match self {
			CollectorKind::ProcTree => "proctree",
			CollectorKind::Cgroup => "cgroup",
		}
	}
}

/// Errors surfaced by the collectors and the probe helpers.
#[derive(Debug)]
pub enum MetricsError {
	Io(std::io::Error),
	CgroupV2Unavailable,
	CgroupPathNotFound(i32),
	MemoryControllerDisabled(String),
	CpuControllerDisabled(String),
	InvalidStatFormat,
	InvalidStatValue(std::num::ParseIntError),
}

impl std::fmt::Display for MetricsError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			MetricsError::Io(e) => write!(f, "metrics io error: {e}"),
			MetricsError::CgroupV2Unavailable => f.write_str("cgroup v2 not available"),
			MetricsError::CgroupPathNotFound(pid) => {
				write!(f, "cgroup v2 path not found for pid {pid}")
			}
			MetricsError::MemoryControllerDisabled(e) => {
				write!(f, "memory controller not enabled for cgroup: {e}")
			}
			MetricsError::CpuControllerDisabled(e) => {
				write!(f, "cpu controller not enabled for cgroup: {e}")
			}
			MetricsError::InvalidStatFormat => f.write_str("invalid /proc/<pid>/stat format"),
			MetricsError::InvalidStatValue(e) => write!(f, "invalid stat value: {e}"),
		}
	}
}

impl std::error::Error for MetricsError {}

impl From<std::io::Error> for MetricsError {
	fn from(e: std::io::Error) -> Self {
		MetricsError::Io(e)
	}
}

impl From<std::num::ParseIntError> for MetricsError {
	fn from(e: std::num::ParseIntError) -> Self {
		MetricsError::InvalidStatValue(e)
	}
}

/// Per-PID entry in a process tree listing. Serialised to JSON over the IPC
/// `tree` command (matches the Go `json` tags byte-for-byte so the wire
/// format does not change).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ChildStat {
	pub pid: i32,
	pub comm: String,
	pub depth: i32,
	pub memory_bytes: i64,
}

#[cfg(all(test, target_os = "linux"))]
mod tests;
