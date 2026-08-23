//! Cgroup V2 metrics collector.
//!
//! Reads `memory.current` and `cpu.stat` from the unified cgroup the target
//! PID belongs to. The CPU usage is the delta of `usage_usec` between two
//! samples, divided by the wall-clock delta; the percentage can exceed
//! `100 * ncpus` because the field is total across cores. Memory is sampled
//! straight off `memory.current`.
//!
//! The collector is `Linux-only`. The factory tries [`ProcTreeCollector`]
//! first because /proc reports per-process memory; the cgroup path is only
//! accurate when the PID runs in a dedicated cgroup (systemd `DynamicUser=`,
//! for instance).

use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

use super::{Collector, Metrics, MetricsError};

/// Default probe location for cgroup v2.
const CGROUP_ROOT: &str = "/sys/fs/cgroup";

/// Collector that pulls memory and CPU usage from the PID's cgroup v2 path.
#[derive(Debug)]
pub struct CgroupCollector {
	pid: i32,
	cgroup_path: PathBuf,
	last_cpu_time: Option<i64>,
	last_sample_time: Option<SystemTime>,
}

/// Build a collector for `pid`. Errors if cgroup v2 is not mounted, the PID
/// has no v2 cgroup, or the controllers we need are disabled.
pub fn new_cgroup_collector(pid: i32) -> Result<CgroupCollector, MetricsError> {
	if !Path::new("/sys/fs/cgroup/cgroup.controllers").exists() {
		return Err(MetricsError::CgroupV2Unavailable);
	}

	let cgroup_path = get_cgroup_path(pid)?;

	if let Err(e) = fs::metadata(cgroup_path.join("memory.current")) {
		return Err(MetricsError::MemoryControllerDisabled(e.to_string()));
	}
	if let Err(e) = fs::metadata(cgroup_path.join("cpu.stat")) {
		return Err(MetricsError::CpuControllerDisabled(e.to_string()));
	}

	Ok(CgroupCollector {
		pid,
		cgroup_path,
		last_cpu_time: None,
		last_sample_time: None,
	})
}

impl Collector for CgroupCollector {
	fn kind(&self) -> &'static str {
		"cgroup"
	}

	fn collect(&mut self) -> Result<Metrics, MetricsError> {
		let now = SystemTime::now();
		let mut m = Metrics {
			timestamp: now,
			memory_bytes: 0,
			cpu_percent: 0.0,
		};

		m.memory_bytes = read_memory(&self.cgroup_path)?;
		let cpu_usage = read_cpu_usage(&self.cgroup_path)?;

		if let (Some(prev_usage), Some(prev_time)) = (self.last_cpu_time, self.last_sample_time) {
			let delta_usage = cpu_usage - prev_usage;
			let delta_micros = now
				.duration_since(prev_time)
				.map(|d| d.as_micros() as i64)
				.unwrap_or(0);
			if delta_micros > 0 {
				m.cpu_percent = (delta_usage as f64 / delta_micros as f64) * 100.0;
			}
		}

		self.last_cpu_time = Some(cpu_usage);
		self.last_sample_time = Some(now);

		Ok(m)
	}
}

impl CgroupCollector {
	/// PID the collector was built for. Exposed for tests and for the
	/// manager's bookkeeping.
	#[must_use]
	pub fn pid(&self) -> i32 {
		self.pid
	}

	/// Resolved cgroup path. Exposed for diagnostics.
	#[must_use]
	pub fn cgroup_path(&self) -> &Path {
		&self.cgroup_path
	}
}

/// Read the cgroup v2 path for `pid` from `/proc/<pid>/cgroup`. Returns the
/// path relative to `/sys/fs/cgroup`. Exposed for tests via the
/// `pub(crate)` visibility so they can exercise the parser without paying
/// for the v2/availability dance that `new_cgroup_collector` runs first.
pub(crate) fn get_cgroup_path(pid: i32) -> Result<PathBuf, MetricsError> {
	let path = format!("/proc/{pid}/cgroup");
	let contents = fs::read_to_string(&path)?;
	for line in contents.lines() {
		// Format: "0::/user.slice/..." — the empty middle field is the
		// v2/unified marker.
		let mut parts = line.splitn(3, ':');
		let _id = parts.next();
		let controllers = parts.next().unwrap_or("");
		let cgroup_rel = parts.next().unwrap_or("");
		if controllers.is_empty() && !cgroup_rel.is_empty() {
			return Ok(PathBuf::from(CGROUP_ROOT).join(cgroup_rel));
		}
	}
	Err(MetricsError::CgroupPathNotFound(pid))
}

fn read_memory(cgroup_path: &Path) -> Result<i64, MetricsError> {
	let bytes = fs::read(cgroup_path.join("memory.current"))?;
	let s = std::str::from_utf8(&bytes).map_err(|e| {
		MetricsError::Io(std::io::Error::other(format!(
			"invalid utf-8 in memory.current: {e}"
		)))
	})?;
	s.trim().parse::<i64>().map_err(MetricsError::from)
}

/// Read `usage_usec` from `<cgroup>/cpu.stat`. Field is microseconds, total
/// across cores.
pub(crate) fn read_cpu_usage(cgroup_path: &Path) -> Result<i64, MetricsError> {
	let contents = fs::read_to_string(cgroup_path.join("cpu.stat"))?;
	for line in contents.lines() {
		if let Some(rest) = line.strip_prefix("usage_usec ") {
			return rest.trim().parse::<i64>().map_err(MetricsError::from);
		}
	}
	Err(MetricsError::InvalidStatFormat)
}
