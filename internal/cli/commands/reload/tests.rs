//! Tests for the reload command.
//!
//! 10 cases ported from `internal/cli/commands/reload/cmd_test.go`.

use std::cell::RefCell;
use std::io;

use crate::cli::commands::reload::{self, Ipc, ReloadResponse};
use crate::ipc::transport::TransportError;
use crate::types::{ProcessInfo, ProcessState};

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

fn empty_proc() -> ProcessInfo {
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

struct MockIpc {
	calls: RefCell<Vec<String>>,
	list_response: Vec<ProcessInfo>,
	reload_response: RefCell<Option<ReloadResponse>>,
	reload_responses: RefCell<Vec<Result<ReloadResponse, ()>>>,
	err: Option<Box<TransportError>>,
}

impl MockIpc {
	fn new() -> Self {
		Self {
			calls: RefCell::new(Vec::new()),
			list_response: Vec::new(),
			reload_response: RefCell::new(None),
			reload_responses: RefCell::new(Vec::new()),
			err: None,
		}
	}

	fn with_list(procs: Vec<ProcessInfo>) -> Self {
		Self {
			calls: RefCell::new(Vec::new()),
			list_response: procs,
			reload_response: RefCell::new(None),
			reload_responses: RefCell::new(Vec::new()),
			err: None,
		}
	}

	fn push_reload(&self, r: Result<ReloadResponse, ()>) {
		self.reload_responses.borrow_mut().push(r);
	}
}

impl Ipc for MockIpc {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError> {
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		Ok(self.list_response.clone())
	}

	fn reload(&mut self, id: &str) -> Result<ReloadResponse, TransportError> {
		self.calls.borrow_mut().push(id.to_string());
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		let mut queue = self.reload_responses.borrow_mut();
		if !queue.is_empty() {
			return match queue.remove(0) {
				Ok(r) => Ok(r),
				Err(()) => Err(TransportError::Io(io::Error::new(
					io::ErrorKind::ConnectionRefused,
					"not found",
				))),
			};
		}
		Ok(self
			.reload_response
			.borrow()
			.clone()
			.unwrap_or(ReloadResponse {
				id: id.to_string(),
				status: "reloaded".to_string(),
			}))
	}
}

fn rebuild_err(e: &TransportError) -> TransportError {
	match e {
		TransportError::Io(io) => TransportError::Io(io::Error::new(io.kind(), format!("{io}"))),
		_ => unreachable!("rebuild_err: variant {:?} not supported", e),
	}
}

#[test]
fn run_missing_args_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = reload::run(None, &mut buf, &[]);
	let err = rc.expect_err("missing args");
	assert!(
		err.to_string().contains("missing process ID or name"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_success_calls_reload() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = reload::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_ipc_error_returns_error() {
	let _g = lock_term();
	let mut client = MockIpc::new();
	client.err = Some(Box::new(TransportError::Io(io::Error::new(
		io::ErrorKind::ConnectionRefused,
		"connection refused",
	))));
	let mut buf = Vec::new();
	let rc = reload::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
	assert!(rc.is_err(), "IPC failure must surface");
}

#[test]
fn run_multiple_ids_makes_n_calls() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = reload::run(
		Some(Box::new(client)),
		&mut buf,
		&["a".to_string(), "b".to_string(), "c".to_string()],
	);
	rc.expect("ok");
}

#[test]
fn get_spec_matches_name() {
	let s = reload::spec();
	assert_eq!(s.name, "reload");
}

#[test]
fn run_json_output_partial_failure() {
	let _g = lock_term();
	let client = MockIpc::new();
	client.push_reload(Ok(ReloadResponse {
		id: "x".into(),
		status: "reloaded".into(),
	}));
	client.push_reload(Err(()));
	client.push_reload(Ok(ReloadResponse {
		id: "x".into(),
		status: "reloaded".into(),
	}));
	let mut buf = Vec::new();
	let rc = reload::run(
		Some(Box::new(client)),
		&mut buf,
		&[
			"a".to_string(),
			"b".to_string(),
			"c".to_string(),
			"--json".to_string(),
		],
	);
	assert!(rc.is_err(), "expected aggregate error");
	let out = String::from_utf8(buf).expect("utf8");
	let decoded: serde_json::Value = serde_json::from_str(&out).expect("json");
	assert_eq!(decoded["summary"]["total"], 3);
	assert_eq!(decoded["summary"]["ok"], 2);
	assert_eq!(decoded["summary"]["failed"], 1);
}

#[test]
fn run_namespace_flag_expands_all_procs_in_ns() {
	let _g = lock_term();
	let client = MockIpc::with_list(vec![
		ProcessInfo {
			id: "id-prod-api".into(),
			name: "api".into(),
			namespace: "prod".into(),
			..empty_proc()
		},
		ProcessInfo {
			id: "id-prod-worker".into(),
			name: "worker".into(),
			namespace: "prod".into(),
			..empty_proc()
		},
		ProcessInfo {
			id: "id-dev-api".into(),
			name: "api".into(),
			namespace: "dev".into(),
			..empty_proc()
		},
	]);
	let mut buf = Vec::new();
	let rc = reload::run(
		Some(Box::new(client)),
		&mut buf,
		&["--namespace".to_string(), "prod".to_string()],
	);
	rc.expect("ok");
}

#[test]
fn run_ns_wildcard_expands_all_procs_in_ns() {
	let _g = lock_term();
	let client = MockIpc::with_list(vec![
		ProcessInfo {
			id: "id-prod-api".into(),
			name: "api".into(),
			namespace: "prod".into(),
			..empty_proc()
		},
		ProcessInfo {
			id: "id-prod-worker".into(),
			name: "worker".into(),
			namespace: "prod".into(),
			..empty_proc()
		},
	]);
	let mut buf = Vec::new();
	let rc = reload::run(Some(Box::new(client)), &mut buf, &["prod:*".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_namespace_flag_rejects_mix_with_positional() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = reload::run(
		Some(Box::new(client)),
		&mut buf,
		&[
			"api".to_string(),
			"--namespace".to_string(),
			"prod".to_string(),
		],
	);
	let err = rc.expect_err("mix must error");
	assert!(
		err.to_string().contains("cannot combine --namespace"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_namespace_flag_empty_namespace_errors() {
	let _g = lock_term();
	let client = MockIpc::with_list(vec![]);
	let mut buf = Vec::new();
	let rc = reload::run(
		Some(Box::new(client)),
		&mut buf,
		&["--namespace".to_string(), "ghost".to_string()],
	);
	assert!(rc.is_err(), "expected empty-namespace error");
}
