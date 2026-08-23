//! Tests for the `_exec-env` wrapper.
//!
//! 6 cases ported from `internal/cli/commands/execenv/cmd_test.go`.

use std::path::PathBuf;

use crate::cli::commands::execenv;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

#[test]
fn load_env_parses_basic_pairs() {
	let _g = lock_term();
	let dir = tempfile::tempdir().expect("tempdir");
	let path: PathBuf = dir.path().join("env");
	let content = "\
# This is a comment
KEY1=value1
KEY2=value2 with spaces
KEY3=value3
EMPTY=
# COMMENTed=ignored
";
	std::fs::write(&path, content).expect("write");

	execenv::load_env(&path).expect("load_env");

	assert_eq!(std::env::var("KEY1").ok().as_deref(), Some("value1"));
	assert_eq!(
		std::env::var("KEY2").ok().as_deref(),
		Some("value2 with spaces")
	);
	assert_eq!(std::env::var("KEY3").ok().as_deref(), Some("value3"));
	assert_eq!(std::env::var("EMPTY").ok().as_deref(), Some(""));

	std::env::remove_var("KEY1");
	std::env::remove_var("KEY2");
	std::env::remove_var("KEY3");
	std::env::remove_var("EMPTY");
}

#[test]
fn load_env_missing_file_errors() {
	let _g = lock_term();
	let err = execenv::load_env(std::path::Path::new("/nonexistent/path/env"))
		.expect_err("missing file must error");
	// The error type carries an io::Error; we don't care about the
	// payload — just that we got a Result::Err.
	let _ = err;
}

#[test]
fn run_no_args_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = execenv::run(&mut buf, &[]);
	let err = rc.expect_err("missing args must error");
	assert!(
		err.to_string().contains("usage:"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_command_not_found_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = execenv::run(
		&mut buf,
		&["this-binary-absolutely-does-not-exist-xyz-123".to_string()],
	);
	let err = rc.expect_err("missing command must error");
	assert!(
		err.to_string().contains("command not found"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_bad_credentials_dir_continues() {
	let _g = lock_term();
	// Point at a credentials dir that doesn't exist; the loader warns and
	// the rest of `Run` proceeds. Then the lookup fails so we still see a
	// command-not-found error.
	unsafe {
		std::env::set_var("CREDENTIALS_DIRECTORY", "/nonexistent/creds/dir");
	}
	let mut buf = Vec::new();
	let rc = execenv::run(
		&mut buf,
		&["this-binary-absolutely-does-not-exist-xyz-123".to_string()],
	);
	let err = rc.expect_err("expected error");
	assert!(
		err.to_string().contains("command not found"),
		"unexpected error: {err}"
	);
	unsafe {
		std::env::remove_var("CREDENTIALS_DIRECTORY");
	}
}

#[test]
fn get_spec_is_hidden_and_named() {
	let s = execenv::spec();
	assert_eq!(s.name, "_exec-env");
	assert!(s.hidden, "spec must be hidden");
	assert!(!s.description.is_empty(), "description must be non-empty");
}
