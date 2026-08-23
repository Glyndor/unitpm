//! Tests for [`authorize_start`]. Mirrors `policy_test.go`.
//!
//! Each case carries the expected outcome (`Ok` / specific rejection) and,
//! where the Go test asserted on a wire code prefix, asserts the same on the
//! Rust side via [`PolicyError`].
//!
//! The Go test uses an external test package (`package policy_test`). The
//! split between production code and test code is the same here — the test
//! module lives in its own file and only imports `super::*` plus the protocol
//! types it needs to construct fixtures.

use crate::ipc::protocol::{AppExec, AppSpec, RunAsPolicy};
use crate::ipc::transport::Identity;

use super::{authorize_start, PolicyError};

fn identity() -> Identity {
	Identity {
		uid: "1000".into(),
		gid: "1000".into(),
		pid: 1234,
	}
}

fn exec_with_shell() -> AppExec {
	AppExec {
		kind: "process".into(),
		command: None,
		args: None,
		entry: None,
		runtime: None,
		shell: true,
	}
}

fn exec_plain() -> AppExec {
	AppExec {
		kind: "process".into(),
		command: None,
		args: None,
		entry: None,
		runtime: None,
		shell: false,
	}
}

fn spec_with_run_as(mode: &str, shell: bool) -> AppSpec {
	AppSpec {
		version: 1,
		id: String::new(),
		name: String::new(),
		namespace: None,
		exec: if shell {
			exec_with_shell()
		} else {
			exec_plain()
		},
		cwd: None,
		env: None,
		env_file: None,
		logs: None,
		restart: None,
		cron: None,
		run_as: Some(Box::new(RunAsPolicy { mode: mode.into() })),
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	}
}

fn spec_no_run_as() -> AppSpec {
	AppSpec {
		version: 1,
		id: String::new(),
		name: String::new(),
		namespace: None,
		exec: exec_plain(),
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

#[test]
fn self_run_allowed_user_daemon() {
	let id = identity();
	let s = spec_with_run_as("self", false);
	assert!(authorize_start(&s, &id, false).is_ok());
}

#[test]
fn shell_in_privileged_daemon_refused() {
	let id = identity();
	let s = spec_with_run_as("self", true);
	let err = authorize_start(&s, &id, true).expect_err("expected refusal");
	assert_eq!(err, PolicyError::ShellNotAllowed);
	assert!(
		err.to_string().starts_with("ERR_UNSUPPORTED"),
		"unexpected message: {err}"
	);
}

#[test]
fn dynamic_in_user_daemon_refused() {
	let id = identity();
	let s = spec_with_run_as("dynamic", false);
	let err = authorize_start(&s, &id, false).expect_err("expected refusal");
	assert_eq!(err, PolicyError::DynamicRequiresSystem);
	assert!(
		err.to_string().starts_with("ERR_UNSUPPORTED"),
		"unexpected message: {err}"
	);
}

#[test]
fn dynamic_in_privileged_daemon_allowed() {
	let id = identity();
	let s = spec_with_run_as("dynamic", false);
	assert!(authorize_start(&s, &id, true).is_ok());
}

#[test]
fn app_user_refused() {
	let id = identity();
	let s = spec_with_run_as("app_user", false);
	let err = authorize_start(&s, &id, false).expect_err("expected refusal");
	assert_eq!(err, PolicyError::ReservedMode("app_user".into()));
	assert!(
		err.to_string().starts_with("ERR_UNSUPPORTED"),
		"unexpected message: {err}"
	);
}

#[test]
fn explicit_user_refused() {
	let id = identity();
	let s = spec_with_run_as("explicit_user", false);
	let err = authorize_start(&s, &id, false).expect_err("expected refusal");
	assert_eq!(err, PolicyError::ReservedMode("explicit_user".into()));
	assert!(
		err.to_string().starts_with("ERR_UNSUPPORTED"),
		"unexpected message: {err}"
	);
}

#[test]
fn invalid_mode_refused() {
	let id = identity();
	let s = spec_with_run_as("invalid", false);
	let err = authorize_start(&s, &id, false).expect_err("expected refusal");
	assert_eq!(err, PolicyError::InvalidMode);
	assert!(
		err.to_string().starts_with("ERR_BAD_REQUEST"),
		"unexpected message: {err}"
	);
}

#[test]
fn run_as_unset_defaults_allowed() {
	let id = identity();
	let s = spec_no_run_as();
	assert!(authorize_start(&s, &id, false).is_ok());
}

#[test]
fn sandbox_allowed_user_daemon() {
	let id = identity();
	let s = spec_with_run_as("sandbox", false);
	assert!(authorize_start(&s, &id, false).is_ok());
}

#[test]
fn sandbox_allowed_system_daemon() {
	let id = identity();
	let s = spec_with_run_as("sandbox", false);
	assert!(authorize_start(&s, &id, true).is_ok());
}

#[test]
fn shell_allowed_user_daemon() {
	let id = identity();
	let s = spec_with_run_as("self", true);
	assert!(authorize_start(&s, &id, false).is_ok());
}
