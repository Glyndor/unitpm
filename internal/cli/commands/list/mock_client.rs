//! Shared fixtures for the `list` command's tests.
//!
//! The MockClient + sample_* helpers used to live in `tests.rs` and
//! were shared by the parser, run, and update test sections. They are
//! pulled out so each test file can `use super::mock_client::*`
//! without a 130-line prelude at the top of every test.

use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};

use crate::cli::commands::list::{IpcError, IpcOps};
use crate::types::{ProcessInfo, ProcessState};

/// In-memory IPC client used by the `run`/`render`/`update` test
/// sections. Each test constructs one with [`MockClient::new`] or
/// [`MockClient::err`], hands it to `run` directly, and asserts on
/// the recorded call count and the response/error returned.
#[derive(Clone, Default)]
pub(crate) struct MockClient {
	pub(crate) procs: Vec<ProcessInfo>,
	pub(crate) err: Option<String>,
	pub(crate) calls: Arc<Mutex<Vec<String>>>,
	pub(crate) fail_count: Arc<AtomicU32>,
}

impl MockClient {
	/// Build a client that returns `procs` from every `call_list`.
	pub(crate) fn new(procs: Vec<ProcessInfo>) -> Self {
		Self {
			procs,
			..Default::default()
		}
	}

	/// Build a client that errors on every `call_list` with `msg`.
	pub(crate) fn err(msg: &str) -> Self {
		Self {
			err: Some(msg.to_string()),
			..Default::default()
		}
	}

	/// Snapshot the recorded IPC call names. Useful for assertions
	/// like "list was called once and never again".
	pub(crate) fn list_calls(&self) -> Vec<String> {
		self.calls.lock().unwrap().clone()
	}
}

impl IpcOps for MockClient {
	fn call_list(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
		self.calls.lock().unwrap().push("list".to_string());
		self.fail_count.fetch_add(1, Ordering::Relaxed);
		if let Some(e) = &self.err {
			return Err(IpcError(e.clone()));
		}
		Ok(self.procs.clone())
	}
}

/// Two representative processes for the sort/filter/render tests.
/// One running with a git branch + dirty flag, one stopped with no
/// extra metadata; they sort differently under every sort spec.
pub(crate) fn sample_procs() -> Vec<ProcessInfo> {
	vec![
		ProcessInfo {
			id: "aaaaaaaa-0000-0000-0000-000000000000".into(),
			name: "z-app".into(),
			namespace: "prod".into(),
			version: "1".into(),
			mode: "fork".into(),
			pid: 1234,
			uptime: 5000,
			restarts: 0,
			state: ProcessState::Running,
			cpu: 1.5,
			memory: 1024 * 1024,
			user: "deploy".into(),
			watch: true,
			git_branch: Some("main".into()),
			git_commit: Some("abc".into()),
			git_dirty: true,
			created_at: Some("2024-01-02T00:00:00Z".into()),
		},
		ProcessInfo {
			id: "bbbbbbbb-0000-0000-0000-000000000000".into(),
			name: "a-app".into(),
			namespace: "staging".into(),
			version: "2".into(),
			mode: "fork".into(),
			pid: 0,
			uptime: 0,
			restarts: 2,
			state: ProcessState::Stopped,
			cpu: 0.0,
			memory: 0,
			user: String::new(),
			watch: false,
			git_branch: None,
			git_commit: None,
			git_dirty: false,
			created_at: Some("2024-01-01T00:00:00Z".into()),
		},
	]
}

/// Empty list, for tests that should not call IPC or render rows.
pub(crate) fn empty_procs() -> Vec<ProcessInfo> {
	Vec::new()
}

/// A Default-initialised `ProcessInfo` — used as the base of
/// `..sample_blank()` spreads where only one or two fields matter.
#[allow(dead_code)]
pub(crate) fn sample_blank() -> ProcessInfo {
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
