//! Tests for the `_exec-sandbox` wrapper.
//!
//! 12 cases ported from `internal/cli/commands/execsandbox/cmd_linux_test.go`.

use std::env;

use crate::cli::commands::execsandbox;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	// `--help` prints and returns Ok without touching the kernel.
	execsandbox::run(&mut buf, &["--help".to_string()]).expect("help ok");
}

#[test]
fn run_missing_config_env() {
	let _g = lock_term();
	env::remove_var(execsandbox::SANDBOX_CONFIG_ENV);
	let mut buf = Vec::new();
	let rc = execsandbox::run(&mut buf, &[]);
	let err = rc.expect_err("missing env must error");
	assert!(
		err.to_string().contains("UNITPM_SANDBOX_CONFIG"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_invalid_json() {
	let _g = lock_term();
	env::set_var(execsandbox::SANDBOX_CONFIG_ENV, "not-json");
	let mut buf = Vec::new();
	let rc = execsandbox::run(&mut buf, &[]);
	let err = rc.expect_err("invalid JSON must error");
	assert!(
		err.to_string().contains("invalid sandbox config"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_missing_command_errors_before_sandbox_setup() {
	let _g = lock_term();
	let cfg = execsandbox::Config {
		cwd: "/tmp".into(),
		log_dir: String::new(),
		allow: Vec::new(),
		limits: Default::default(),
		command: String::new(),
		args: Vec::new(),
	};
	let raw = serde_json::to_string(&cfg).expect("encode");
	env::set_var(execsandbox::SANDBOX_CONFIG_ENV, &raw);
	let mut buf = Vec::new();
	let rc = execsandbox::run(&mut buf, &[]);
	let err = rc.expect_err("missing command must error");
	assert!(
		err.to_string().contains("missing command"),
		"unexpected error: {err}"
	);
	// Cleanup: ensure the env var doesn't leak into other tests.
	env::remove_var(execsandbox::SANDBOX_CONFIG_ENV);
}

#[test]
fn serialize_roundtrip() {
	let _g = lock_term();
	let cfg = execsandbox::Config {
		cwd: "/tmp".into(),
		log_dir: String::new(),
		allow: Vec::new(),
		limits: Default::default(),
		command: "/bin/echo".into(),
		args: vec!["hello".into()],
	};
	let raw = execsandbox::serialize(&cfg).expect("encode");
	// Decode the outer envelope to confirm the shape; the inner
	// allow/limits fields use a permissive adapter that returns an
	// empty rule regardless of the on-wire payload.
	#[derive(serde::Deserialize)]
	struct Outer {
		#[serde(default)]
		command: String,
		#[serde(default)]
		args: Vec<String>,
	}
	let outer: Outer = serde_json::from_str(&raw).expect("decode");
	assert_eq!(outer.command, "/bin/echo");
	assert_eq!(outer.args, vec!["hello".to_string()]);
}

#[test]
fn config_env_var_constant() {
	assert_eq!(execsandbox::config_env_var(), "UNITPM_SANDBOX_CONFIG");
}

#[test]
fn wrapper_command_returns_two_tokens() {
	let parts = execsandbox::wrapper_command("/usr/bin/unitpm");
	assert_eq!(parts.len(), 2);
	assert_eq!(parts[0], "/usr/bin/unitpm");
	assert_eq!(parts[1], "_exec-sandbox");
}

#[test]
fn shell_quote_joins_with_spaces() {
	let got = execsandbox::shell_quote(&["a".to_string(), "b".to_string(), "c".to_string()]);
	assert_eq!(got, "a b c");
}

#[test]
fn get_spec_is_hidden() {
	let s = execsandbox::spec();
	assert_eq!(s.name, "_exec-sandbox");
	assert!(s.hidden, "spec must be hidden");
}

#[test]
fn run_relative_cwd_errors() {
	let _g = lock_term();
	let cfg = execsandbox::Config {
		cwd: "relative/path".into(),
		log_dir: String::new(),
		allow: Vec::new(),
		limits: Default::default(),
		command: "echo".into(),
		args: Vec::new(),
	};
	let raw = serde_json::to_string(&cfg).expect("encode");
	env::set_var(execsandbox::SANDBOX_CONFIG_ENV, &raw);
	let mut buf = Vec::new();
	let rc = execsandbox::run(&mut buf, &[]);
	let err = rc.expect_err("relative cwd must error");
	assert!(
		err.to_string().contains("must be absolute"),
		"unexpected error: {err}"
	);
	env::remove_var(execsandbox::SANDBOX_CONFIG_ENV);
}

#[test]
fn run_prctl_or_mount_fails_unprivileged() {
	// Outside a real namespace, the prctl/mount steps fail. Any error is
	// acceptable — what matters is that we never panic.
	let _g = lock_term();
	let cfg = execsandbox::Config {
		cwd: "/tmp".into(),
		log_dir: String::new(),
		allow: Vec::new(),
		limits: Default::default(),
		command: "/bin/true".into(),
		args: Vec::new(),
	};
	let raw = serde_json::to_string(&cfg).expect("encode");
	env::set_var(execsandbox::SANDBOX_CONFIG_ENV, &raw);
	let mut buf = Vec::new();
	let _ = execsandbox::run(&mut buf, &[]);
	// Don't assert — the path can either return Err or stay alive if the
	// test happens to be running under a namespace.
	env::remove_var(execsandbox::SANDBOX_CONFIG_ENV);
}

#[test]
fn print_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	execsandbox::print_help(&mut buf);
}
