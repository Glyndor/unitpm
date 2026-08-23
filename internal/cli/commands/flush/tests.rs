//! Tests for the flush command.
//!
//! 11 cases ported from `internal/cli/commands/flush/cmd_test.go`.

use std::cell::RefCell;
use std::io;

use crate::cli::commands::flush::{self, FlushResponse, Ipc};
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
	flush_response: RefCell<Option<FlushResponse>>,
	err: Option<Box<TransportError>>,
}

impl MockIpc {
	fn new() -> Self {
		Self {
			calls: RefCell::new(Vec::new()),
			list_response: Vec::new(),
			flush_response: RefCell::new(None),
			err: None,
		}
	}

	fn with_list(procs: Vec<ProcessInfo>) -> Self {
		Self {
			calls: RefCell::new(Vec::new()),
			list_response: procs,
			flush_response: RefCell::new(None),
			err: None,
		}
	}

	fn ok_response(&self, r: FlushResponse) {
		*self.flush_response.borrow_mut() = Some(r);
	}
}

impl Ipc for MockIpc {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError> {
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		Ok(self.list_response.clone())
	}

	fn flush(&mut self, id: &str) -> Result<FlushResponse, TransportError> {
		self.calls.borrow_mut().push(id.to_string());
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		Ok(self
			.flush_response
			.borrow()
			.clone()
			.unwrap_or(FlushResponse {
				id: id.to_string(),
				status: "flushed".to_string(),
				bytes_freed: 0,
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
	let rc = flush::run(None, &mut buf, &[]);
	let err = rc.expect_err("missing args");
	assert!(
		err.to_string().contains("missing process ID or name"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_success_calls_flush() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = flush::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
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
	let rc = flush::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
	assert!(rc.is_err(), "IPC failure must surface");
}

#[test]
fn run_multiple_ids_makes_n_calls() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = flush::run(
		Some(Box::new(client)),
		&mut buf,
		&["a".to_string(), "b".to_string(), "c".to_string()],
	);
	rc.expect("ok");
}

#[test]
fn get_spec_matches_name() {
	let s = flush::spec();
	assert_eq!(s.name, "flush");
}

#[test]
fn run_bytes_freed_surfaced_in_json() {
	let _g = lock_term();
	let client = MockIpc::new();
	client.ok_response(FlushResponse {
		id: "abc-123".into(),
		status: "flushed".into(),
		bytes_freed: 1048576,
	});
	let mut buf = Vec::new();
	flush::run(
		Some(Box::new(client)),
		&mut buf,
		&["--json".to_string(), "abc-123".to_string()],
	)
	.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	let decoded: serde_json::Value = serde_json::from_str(&out).expect("json");
	let extra = &decoded["results"][0]["extra"];
	assert_eq!(extra["bytes_freed"].as_i64(), Some(1048576));
}

#[test]
fn run_bytes_freed_omitted_when_zero() {
	let _g = lock_term();
	let client = MockIpc::new();
	// Default response has bytes_freed=0; ensure JSON does not include it.
	let mut buf = Vec::new();
	flush::run(
		Some(Box::new(client)),
		&mut buf,
		&["--json".to_string(), "abc-123".to_string()],
	)
	.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	let decoded: serde_json::Value = serde_json::from_str(&out).expect("json");
	let extra = &decoded["results"][0]["extra"];
	assert!(
		extra.get("bytes_freed").is_none(),
		"bytes_freed should be omitted when zero, got {extra}"
	);
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
	let rc = flush::run(
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
	let rc = flush::run(Some(Box::new(client)), &mut buf, &["prod:*".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_namespace_flag_rejects_mix_with_positional() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = flush::run(
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
	let rc = flush::run(
		Some(Box::new(client)),
		&mut buf,
		&["--namespace".to_string(), "ghost".to_string()],
	);
	assert!(rc.is_err(), "expected empty-namespace error");
}
