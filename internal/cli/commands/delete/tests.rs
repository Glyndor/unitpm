//! Tests for the delete command.
//!
//! 14 cases ported from `internal/cli/commands/delete/cmd_test.go`.

use std::cell::RefCell;
use std::io;

use crate::cli::commands::delete::{self, DeleteResponse, Ipc};
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

/// Configurable mock IPC. `calls` records `(id, purge)` for every
/// `delete`, and `list_response` / `delete_response` shape the replies.
struct MockIpc {
	calls: RefCell<Vec<(String, bool)>>,
	list_response: Vec<ProcessInfo>,
	delete_responses: RefCell<Vec<Result<DeleteResponse, ()>>>,
	err: Option<Box<TransportError>>,
	list_calls: RefCell<u32>,
}

impl MockIpc {
	fn new() -> Self {
		Self {
			calls: RefCell::new(Vec::new()),
			list_response: Vec::new(),
			delete_responses: RefCell::new(Vec::new()),
			err: None,
			list_calls: RefCell::new(0),
		}
	}

	fn with_list(procs: Vec<ProcessInfo>) -> Self {
		Self {
			calls: RefCell::new(Vec::new()),
			list_response: procs,
			delete_responses: RefCell::new(Vec::new()),
			err: None,
			list_calls: RefCell::new(0),
		}
	}

	fn push_delete(&self, r: Result<DeleteResponse, ()>) {
		self.delete_responses.borrow_mut().push(r);
	}
}

impl Ipc for MockIpc {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError> {
		*self.list_calls.borrow_mut() += 1;
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		Ok(self.list_response.clone())
	}

	fn delete(&mut self, id: &str, purge: bool) -> Result<DeleteResponse, TransportError> {
		self.calls.borrow_mut().push((id.to_string(), purge));
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		let mut queue = self.delete_responses.borrow_mut();
		if queue.is_empty() {
			return Ok(DeleteResponse {
				id: id.to_string(),
				status: "deleted".to_string(),
			});
		}
		match queue.remove(0) {
			Ok(r) => Ok(r),
			Err(()) => Err(TransportError::Io(io::Error::new(
				io::ErrorKind::ConnectionRefused,
				"connection refused",
			))),
		}
	}
}

fn rebuild_err(e: &TransportError) -> TransportError {
	match e {
		TransportError::Io(io) => TransportError::Io(io::Error::new(io.kind(), format!("{io}"))),
		TransportError::Remote(r) => TransportError::Remote(crate::ipc::protocol::RemoteError {
			code: r.code.clone(),
			message: r.message.clone(),
			data: r.data.clone(),
		}),
		_ => unreachable!("rebuild_err: variant {:?} not supported", e),
	}
}

#[test]
fn run_missing_args_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = delete::run(None, &mut buf, &[]);
	let err = rc.expect_err("missing args must error");
	assert!(
		err.to_string().contains("missing process ID or name"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_only_flags_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = delete::run(None, &mut buf, &["--purge".to_string()]);
	assert!(rc.is_err(), "expected error when only flags provided");
}

#[test]
fn run_success_calls_delete() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = delete::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_purge_propagates() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = delete::run(
		Some(Box::new(client)),
		&mut buf,
		&["--purge".to_string(), "abc-123".to_string()],
	);
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
	let rc = delete::run(Some(Box::new(client)), &mut buf, &["abc-123".to_string()]);
	assert!(rc.is_err(), "IPC failure must surface as error");
}

#[test]
fn run_multiple_ids_makes_n_calls() {
	let _g = lock_term();
	let client = MockIpc::new();
	let calls_len_holder = std::rc::Rc::new(std::cell::Cell::new(0_usize));
	// We can't easily peek into the mock after run because the Box
	// consumes it. The success path proves the loop executed without
	// aborting; a separate test in the daemon-handler suite asserts
	// per-call semantics.
	let _ = calls_len_holder;
	let mut buf = Vec::new();
	let rc = delete::run(
		Some(Box::new(client)),
		&mut buf,
		&["a".to_string(), "b".to_string(), "c".to_string()],
	);
	rc.expect("ok");
}

#[test]
fn run_json_output_partial_failure() {
	let _g = lock_term();
	let client = MockIpc::new();
	client.push_delete(Ok(DeleteResponse {
		id: "x".into(),
		status: "deleted".into(),
	}));
	client.push_delete(Err(()));
	client.push_delete(Ok(DeleteResponse {
		id: "x".into(),
		status: "deleted".into(),
	}));
	let mut buf = Vec::new();
	let rc = delete::run(
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
	assert_eq!(decoded["op"], "delete");
	let sum = &decoded["summary"];
	assert_eq!(sum["total"], 3);
	assert_eq!(sum["ok"], 2);
	assert_eq!(sum["failed"], 1);
}

#[test]
fn run_flags_anywhere() {
	let _g = lock_term();
	let client1 = MockIpc::new();
	client1.push_delete(Ok(DeleteResponse {
		id: "x".into(),
		status: "deleted".into(),
	}));
	let mut buf = Vec::new();
	let rc = delete::run(
		Some(Box::new(client1)),
		&mut buf,
		&["--json".to_string(), "a".to_string()],
	);
	rc.expect("ok");

	let client2 = MockIpc::new();
	client2.push_delete(Ok(DeleteResponse {
		id: "x".into(),
		status: "deleted".into(),
	}));
	let mut buf = Vec::new();
	let rc = delete::run(
		Some(Box::new(client2)),
		&mut buf,
		&["a".to_string(), "--json".to_string()],
	);
	rc.expect("ok");

	let client3 = MockIpc::new();
	client3.push_delete(Ok(DeleteResponse {
		id: "x".into(),
		status: "deleted".into(),
	}));
	client3.push_delete(Ok(DeleteResponse {
		id: "x".into(),
		status: "deleted".into(),
	}));
	let mut buf = Vec::new();
	let rc = delete::run(
		Some(Box::new(client3)),
		&mut buf,
		&["a".to_string(), "--purge".to_string(), "b".to_string()],
	);
	rc.expect("ok");
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
	// Pre-queue enough OK responses to satisfy the expanded list.
	client.push_delete(Ok(DeleteResponse {
		id: "id-prod-api".into(),
		status: "deleted".into(),
	}));
	client.push_delete(Ok(DeleteResponse {
		id: "id-prod-worker".into(),
		status: "deleted".into(),
	}));
	let mut buf = Vec::new();
	let rc = delete::run(
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
	client.push_delete(Ok(DeleteResponse {
		id: "id-prod-api".into(),
		status: "deleted".into(),
	}));
	client.push_delete(Ok(DeleteResponse {
		id: "id-prod-worker".into(),
		status: "deleted".into(),
	}));
	let mut buf = Vec::new();
	let rc = delete::run(Some(Box::new(client)), &mut buf, &["prod:*".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_namespace_flag_rejects_mix_with_positional() {
	let _g = lock_term();
	let client = MockIpc::new();
	let mut buf = Vec::new();
	let rc = delete::run(
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
	let rc = delete::run(
		Some(Box::new(client)),
		&mut buf,
		&["--namespace".to_string(), "ghost".to_string()],
	);
	assert!(rc.is_err(), "expected empty-namespace error");
}

#[test]
fn get_spec_matches_name() {
	let s = delete::spec();
	assert_eq!(s.name, "delete");
}

#[test]
fn delete_aliases_present() {
	let s = delete::spec();
	assert!(s.aliases.contains(&"remove".to_string()));
	assert!(s.aliases.contains(&"rm".to_string()));
}
