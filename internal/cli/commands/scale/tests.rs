//! Tests for the scale command.
//!
//! 10 cases ported from `internal/cli/commands/scale/cmd_test.go`.

use std::io;

use crate::cli::commands::scale::{self, Ipc};
use crate::ipc::protocol::ScaleResponse;
use crate::ipc::transport::TransportError;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

/// Mock that records every (name, namespace, target) tuple the command
/// sends, plus a configurable response or error.
struct MockIpc {
	calls: std::cell::RefCell<Vec<(String, String, i32)>>,
	response: Option<ScaleResponse>,
	err: Option<Box<TransportError>>,
}

impl MockIpc {
	fn ok(response: ScaleResponse) -> Self {
		Self {
			calls: std::cell::RefCell::new(Vec::new()),
			response: Some(response),
			err: None,
		}
	}

	fn err(err: TransportError) -> Self {
		Self {
			calls: std::cell::RefCell::new(Vec::new()),
			response: None,
			err: Some(Box::new(err)),
		}
	}
}

impl Ipc for MockIpc {
	fn scale(
		&mut self,
		name: &str,
		namespace: &str,
		target: i32,
	) -> Result<ScaleResponse, TransportError> {
		self.calls
			.borrow_mut()
			.push((name.to_string(), namespace.to_string(), target));
		if let Some(e) = self.err.as_deref() {
			return Err(rebuild_err(e));
		}
		Ok(self.response.clone().expect("configured response"))
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
	let rc = scale::run(None, &mut buf, &[]);
	let err = rc.expect_err("missing args");
	assert!(
		err.to_string().contains("usage:"),
		"unexpected error: {err}"
	);

	let mut buf = Vec::new();
	let rc = scale::run(None, &mut buf, &["onlyname".to_string()]);
	assert!(rc.is_err(), "expected usage error with only one arg");
}

#[test]
fn run_bad_count_errors() {
	let _g = lock_term();
	for bad in ["abc", "-1", "1.5"] {
		let mut buf = Vec::new();
		let rc = scale::run(None, &mut buf, &["worker".to_string(), bad.to_string()]);
		assert!(rc.is_err(), "target {bad:?} should be rejected");
	}
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	scale::run(None, &mut buf, &["--help".to_string()]).expect("ok");
}

#[test]
fn run_success_calls_scale() {
	let _g = lock_term();
	let resp = ScaleResponse {
		base_name: "worker".into(),
		namespace: "default".into(),
		before: 2,
		after: 5,
		created: Some(vec![
			"worker-3".into(),
			"worker-4".into(),
			"worker-5".into(),
		]),
		deleted: None,
	};
	let client = MockIpc::ok(resp);
	let mut buf = Vec::new();
	scale::run(
		Some(Box::new(client)),
		&mut buf,
		&["worker".to_string(), "5".to_string()],
	)
	.expect("ok");
}

#[test]
fn run_namespace_qualified() {
	let _g = lock_term();
	let resp = ScaleResponse {
		base_name: "api".into(),
		namespace: "prod".into(),
		before: 1,
		after: 3,
		created: None,
		deleted: None,
	};
	let client = MockIpc::ok(resp);
	let mut buf = Vec::new();
	scale::run(
		Some(Box::new(client)),
		&mut buf,
		&["prod:api".to_string(), "3".to_string()],
	)
	.expect("ok");
}

#[test]
fn run_ipc_error_propagates() {
	let _g = lock_term();
	let client = MockIpc::err(TransportError::Io(io::Error::new(
		io::ErrorKind::NotFound,
		"not found",
	)));
	let mut buf = Vec::new();
	let rc = scale::run(
		Some(Box::new(client)),
		&mut buf,
		&["worker".to_string(), "2".to_string()],
	);
	let err = rc.expect_err("expected error");
	assert!(
		err.to_string().contains("scale failed"),
		"unexpected error: {err}"
	);
}

#[test]
fn get_spec_matches_name() {
	let s = scale::spec();
	assert_eq!(s.name, "scale");
}

#[test]
fn print_help_does_not_panic() {
	let _g = lock_term();
	scale::print_help(&mut Vec::new());
}

#[test]
fn run_json_output() {
	let _g = lock_term();
	let resp = ScaleResponse {
		base_name: "worker".into(),
		namespace: "default".into(),
		before: 2,
		after: 4,
		created: Some(vec!["worker-3".into(), "worker-4".into()]),
		deleted: None,
	};
	let client = MockIpc::ok(resp);
	let mut buf = Vec::new();
	scale::run(
		Some(Box::new(client)),
		&mut buf,
		&["worker".to_string(), "4".to_string(), "--json".to_string()],
	)
	.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	let decoded: serde_json::Value = serde_json::from_str(&out).expect("json");
	assert_eq!(decoded["base_name"], "worker");
	assert_eq!(decoded["before"], 2);
	assert_eq!(decoded["after"], 4);
	assert_eq!(decoded["created"][0], "worker-3");
	assert_eq!(decoded["created"][1], "worker-4");
}

#[test]
fn run_flag_after_positionals() {
	let _g = lock_term();
	let resp = ScaleResponse {
		base_name: "worker".into(),
		namespace: "default".into(),
		before: 1,
		after: 2,
		created: None,
		deleted: None,
	};
	let client = MockIpc::ok(resp);
	let mut buf = Vec::new();
	scale::run(
		Some(Box::new(client)),
		&mut buf,
		&["worker".to_string(), "2".to_string(), "--json".to_string()],
	)
	.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(
		out.starts_with('{'),
		"--json at the end should still be honored; got: {out}"
	);
}
