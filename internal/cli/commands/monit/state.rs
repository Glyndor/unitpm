//! `monit`'s render-time state.

use std::time::Duration;

use crate::metrics::ChildStat;
use crate::types::{ProcessInfo, ProcessState};

/// Maximum samples kept in `cpu_hist` and `mem_hist`. The Go reference
/// uses the same value.
pub const MAX_HISTORY: usize = 120;

/// Refresh interval. Drives the loop's `Tick` event cadence.
pub const REFRESH_RATE: Duration = Duration::from_secs(1);

/// Cumulative state shared between the loop and the renderer.
#[derive(Debug, Clone)]
pub struct MonitState {
	pub info: ProcessInfo,
	pub spec: crate::ipc::protocol::AppSpec,
	pub tree: Vec<ChildStat>,
	pub cpu_hist: Vec<f64>,
	pub mem_hist: Vec<i64>,
	pub mem_max: i64,
}

impl Default for MonitState {
	fn default() -> Self {
		Self {
			info: ProcessInfo {
				id: String::new(),
				name: String::new(),
				namespace: String::new(),
				version: String::new(),
				mode: String::new(),
				pid: 0,
				uptime: 0,
				restarts: 0,
				state: ProcessState::Running,
				cpu: 0.0,
				memory: 0,
				user: String::new(),
				watch: false,
				git_branch: None,
				git_commit: None,
				git_dirty: false,
				created_at: None,
			},
			spec: empty_spec(),
			tree: Vec::new(),
			cpu_hist: Vec::new(),
			mem_hist: Vec::new(),
			mem_max: 0,
		}
	}
}

fn empty_spec() -> crate::ipc::protocol::AppSpec {
	crate::ipc::protocol::AppSpec {
		version: 1,
		id: String::new(),
		name: String::new(),
		namespace: None,
		exec: crate::ipc::protocol::AppExec {
			kind: String::new(),
			command: None,
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: None,
		env: None,
		env_file: None,
		logs: None,
		restart: None,
		cron: None,
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	}
}
