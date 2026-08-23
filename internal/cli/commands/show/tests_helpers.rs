//! Test fixtures for the `show` command.
//!
//! Shared between [`mod`](super) and [`render::tests`](super::render::tests)
//! so the render tests can build a fully-formed `AppSpec` without
//! duplicating the scaffolding.

use crate::ipc::protocol::{AppLogs, AppSpec};
use crate::types::{ProcessInfo, ProcessState};

use super::empty_spec;

#[must_use]
pub fn spec_with_logs() -> AppSpec {
	AppSpec {
		logs: Some(Box::new(AppLogs {
			mode: "file".into(),
			dir: Some("/var/log".into()),
			stdout: Some("out.log".into()),
			stderr: Some("err.log".into()),
			format: None,
			timestamp: None,
		})),
		..empty_spec()
	}
}

#[must_use]
pub fn empty_info() -> ProcessInfo {
	ProcessInfo {
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
	}
}
