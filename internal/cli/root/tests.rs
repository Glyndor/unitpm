//! Tests for the root dispatcher.
//!
//! 9 cases ported from `internal/cli/root/{root_test.go, root_internal_test.go}`.

use crate::cli::errs::UsageError;
use crate::cli::root;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

#[test]
fn execute_help_variants_return_zero() {
	let _g = lock_term();
	let mut out = Vec::new();
	let mut err = Vec::new();
	assert_eq!(
		root::execute_with(&["help".to_string()], &mut out, &mut err),
		0
	);
	assert_eq!(
		root::execute_with(&["--help".to_string()], &mut out, &mut err),
		0
	);
	assert_eq!(
		root::execute_with(&["-h".to_string()], &mut out, &mut err),
		0
	);
}

#[test]
fn execute_unknown_command_returns_one() {
	let _g = lock_term();
	let mut out = Vec::new();
	let mut err = Vec::new();
	assert_eq!(
		root::execute_with(&["unknown-command".to_string()], &mut out, &mut err),
		1
	);
	assert!(!err.is_empty(), "unknown command should write to stderr");
}

#[test]
fn is_help_request_true_for_recognised_flags() {
	let cases: &[&[&str]] = &[
		&["-h"],
		&["--help"],
		&["start", "-h"],
		&["--help", "something"],
		&["foo", "--help", "bar"],
	];
	for case in cases {
		let args: Vec<String> = case.iter().map(|s| (*s).to_string()).collect();
		assert!(
			root::is_help_request(&args),
			"is_help_request({args:?}) should be true"
		);
	}
}

#[test]
fn is_help_request_false_for_non_help_args() {
	let cases: &[&[&str]] = &[
		&[],
		&["start"],
		&["start", "--name", "api"],
		&["-help"],
		&["help"],
	];
	for case in cases {
		let args: Vec<String> = case.iter().map(|s| (*s).to_string()).collect();
		assert!(
			!root::is_help_request(&args),
			"is_help_request({args:?}) should be false"
		);
	}
}

#[test]
fn handle_error_usage_error_does_not_panic() {
	let _g = lock_term();
	let err: Box<dyn std::error::Error> = Box::new(UsageError::new("missing required flag --name"));
	let mut out = Vec::new();
	let mut err_out = Vec::new();
	root::handle_error_to(err, "start", &mut out, &mut err_out);
}

#[test]
fn handle_error_generic_error_does_not_panic() {
	#[derive(Debug)]
	struct TestError(&'static str);
	impl std::fmt::Display for TestError {
		fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
			f.write_str(self.0)
		}
	}
	impl std::error::Error for TestError {}
	let _g = lock_term();
	let err: Box<dyn std::error::Error> = Box::new(TestError("daemon not running"));
	let mut out = Vec::new();
	let mut err_out = Vec::new();
	root::handle_error_to(err, "list", &mut out, &mut err_out);
}

#[test]
fn print_command_help_unknown_returns_zero() {
	let _g = lock_term();
	let mut buf = Vec::new();
	assert_eq!(
		root::print_command_help_to("unknown-xyz-command", &mut buf),
		0
	);
}

#[test]
fn print_command_help_known_returns_zero() {
	// Phase 6d covers a subset; the others remain on the stub roster
	// but render the same kind of "not yet ported" help block, which
	// `print_command_help_to` accepts.
	let known = [
		root::cmd::LIST,
		root::cmd::START,
		root::cmd::STOP,
		root::cmd::RESTART,
		root::cmd::DELETE,
		root::cmd::LOGS,
		root::cmd::VERSION,
		root::cmd::APPLY,
		root::cmd::FLUSH,
		root::cmd::UPDATE,
	];
	let _g = lock_term();
	for name in known {
		let mut buf = Vec::new();
		assert_eq!(
			root::print_command_help_to(name, &mut buf),
			0,
			"{name} returned non-zero"
		);
	}
}

#[test]
fn resolve_command_known() {
	let _g = lock_term();
	// `execute` registers every command before dispatching. Tests of
	// `resolve_command` must do the same; otherwise the registry can
	// be empty (e.g. when this test runs in isolation before another
	// test triggers `execute`).
	crate::cli::root::register_all();
	let (name, hit) = crate::cli::registry::resolve("apply");
	assert!(hit);
	assert_eq!(name, "apply");
}

#[test]
fn resolve_command_unknown() {
	let _g = lock_term();
	crate::cli::root::register_all();
	let (_name, hit) = crate::cli::registry::resolve("definitely-not-a-command");
	assert!(!hit);
}

#[test]
fn apply_global_flags_quiet_consumed() {
	let _g = crate::term::tests::lock_term();
	crate::term::set_quiet(false);
	let args = vec!["start".to_string(), "--quiet".to_string(), "-q".to_string()];
	let stripped = root::apply_global_flags(&args);
	assert_eq!(stripped, vec!["start".to_string()]);
	assert!(crate::term::is_quiet());
}
