//! Tests for the reset command.
//!
//! 11 cases ported from `internal/cli/commands/reset/cmd_test.go`.

use std::cell::RefCell;
use std::io;

use crate::cli::commands::reset::{self, Ipc, ResetResponse};
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
	list_response: Vec<ProcessInfo>,
	reset_responses: RefCell<Vec<Result<ResetResponse, ()>>>,
	err: Option<Box<TransportError>>,
}

impl MockIpc {
	fn new() -> Self {
		Self {
			list_response: Vec::new(),
			reset_responses: RefCell::new(Vec::new()),
			err: None,
		}
	}

	fn with_list(procs: Vec<ProcessInfo>) -> Self {
		Self {
			list_response: procs,
			reset_responses: RefCell::new(Vec::new()),
			err: None,
		}
	}

	fn push_reset(&self, r: Result<ResetResponse, ()>) {
		self.reset_responses.borrow_mut().push(r);
	}
}

impl Ipc for MockIpc {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError> {
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		Ok(self.list_response.clone())
	}

	fn reset(&mut self, id: &str) -> Result<ResetResponse, TransportError> {
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		let mut queue = self.reset_responses.borrow_mut();
		if !queue.is_empty() {
			return match queue.remove(0) {
				Ok(r) => Ok(r),
				Err(()) => Err(TransportError::Io(io::Error::new(
					io::ErrorKind::NotFound,
					"not found",
				))),
			};
		}
		Ok(ResetResponse {
			id: id.to_string(),
			status: "reset".to_string(),
		})
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
	let rc = reset::run(None, &mut buf, &[]);
	let err = rc.expect_err("missing args");
	assert!(
		err.to_string().contains("missing process ID or name"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_success_calls_reset() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = reset::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_multiple_ids_makes_n_calls() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = reset::run(
		Some(Box::new(client)),
		&mut buf,
		&["a".to_string(), "b".to_string(), "c".to_string()],
	);
	rc.expect("ok");
}

#[test]
fn run_ipc_error_returns_error() {
	let _g = lock_term();
	let mut client = MockIpc::new();
	client.err = Some(Box::new(TransportError::Io(io::Error::new(
		io::ErrorKind::NotFound,
		"not found",
	))));
	let mut buf = Vec::new();
	let rc = reset::run(Some(Box::new(client)), &mut buf, &["ghost".to_string()]);
	let err = rc.expect_err("expected error");
	assert!(
		err.to_string().contains("reset"),
		"expected error to mention op, got {err}"
	);
}

#[test]
fn run_partial_failure() {
	let _g = lock_term();
	let client = MockIpc::new();
	client.push_reset(Ok(ResetResponse {
		id: "ok-id".into(),
		status: "reset".into(),
	}));
	client.push_reset(Err(()));
	let mut buf = Vec::new();
	let rc = reset::run(
		Some(Box::new(client)),
		&mut buf,
		&["a".to_string(), "b".to_string()],
	);
	let err = rc.expect_err("expected aggregate error");
	assert!(
		err.to_string().contains("1 of 2"),
		"expected '1 of 2', got {err}"
	);
}

#[test]
fn get_spec_matches_name() {
	let s = reset::spec();
	assert_eq!(s.name, "reset");
}

#[test]
fn print_help_does_not_panic() {
	let _g = lock_term();
	reset::print_help(&mut Vec::new());
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
	let rc = reset::run(
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
	let rc = reset::run(Some(Box::new(client)), &mut buf, &["prod:*".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_namespace_flag_rejects_mix_with_positional() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = reset::run(
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
	let rc = reset::run(
		Some(Box::new(client)),
		&mut buf,
		&["--namespace".to_string(), "ghost".to_string()],
	);
	assert!(rc.is_err(), "expected empty-namespace error");
}
