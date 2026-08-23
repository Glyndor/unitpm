//! Tests for [`crate::daemon::manager::spawn`]. Mirrors
//! `process_test.go` and the relevant parts of `manager_test.go`.

use super::*;
use crate::daemon::manager::spawn::{resolve_command, shell_quote};
use crate::ipc::protocol::{AppExec, AppSpec, RunAsPolicy};

#[test]
fn shell_quote_round_trip() {
	let cases: [(&str, &str); 10] = [
		("hello", "'hello'"),
		("hello world", "'hello world'"),
		("it's", "'it'\\''s'"),
		("$(rm -rf /)", "'$(rm -rf /)'"),
		("`whoami`", "'`whoami`'"),
		("foo;bar", "'foo;bar'"),
		("a|b&&c", "'a|b&&c'"),
		("", "''"),
		("normal-file.js", "'normal-file.js'"),
		("\"quoted\"", "'\"quoted\"'"),
	];
	for (input, want) in cases {
		let got = shell_quote(input);
		assert_eq!(got, want, "shellQuote({input:?}) = {got:?}, want {want:?}");
	}
}

fn minimal_spec(id: &str, exec: AppExec) -> AppSpec {
	AppSpec {
		version: 1,
		id: id.into(),
		name: "n".into(),
		namespace: None,
		exec,
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
fn resolve_command_command_kind() {
	let spec = minimal_spec(
		"x",
		AppExec {
			kind: "command".into(),
			command: Some("/bin/sleep".into()),
			args: Some(vec!["10".into()]),
			entry: None,
			runtime: None,
			shell: false,
		},
	);
	let (bin, args) = resolve_command(&spec).unwrap();
	assert_eq!(bin, "/bin/sleep");
	assert_eq!(args, vec!["10"]);
}

#[test]
fn resolve_command_entry_kind() {
	let spec = minimal_spec(
		"x",
		AppExec {
			kind: "entry".into(),
			command: None,
			args: Some(vec!["--flag".into()]),
			entry: Some("./app.js".into()),
			runtime: Some("node --harmony".into()),
			shell: false,
		},
	);
	let (bin, args) = resolve_command(&spec).unwrap();
	assert_eq!(bin, "node");
	assert_eq!(args, vec!["--harmony", "./app.js", "--flag"]);
}

#[test]
fn resolve_command_entry_requires_both() {
	let spec = minimal_spec(
		"x",
		AppExec {
			kind: "entry".into(),
			command: None,
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
	);
	assert!(matches!(
		resolve_command(&spec).unwrap_err(),
		SpawnError::EntryAndRuntimeRequired
	));
}

#[test]
fn resolve_command_unknown_kind() {
	let spec = minimal_spec(
		"x",
		AppExec {
			kind: "shell".into(),
			command: None,
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
	);
	assert!(matches!(
		resolve_command(&spec).unwrap_err(),
		SpawnError::InvalidExecType
	));
}

#[test]
fn resolve_command_too_many_args() {
	let spec = minimal_spec(
		"x",
		AppExec {
			kind: "command".into(),
			command: Some("echo".into()),
			args: Some(vec!["a".into(); 257]),
			entry: None,
			runtime: None,
			shell: false,
		},
	);
	assert!(matches!(
		resolve_command(&spec).unwrap_err(),
		SpawnError::TooManyArguments
	));
}

#[test]
fn prepare_env_dynamic_strips_home() {
	let spec = minimal_spec(
		"123e4567-e89b-12d3-a456-426614174000",
		AppExec {
			kind: "command".into(),
			command: Some("echo".into()),
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
	);
	let mut spec = spec;
	spec.run_as = Some(Box::new(RunAsPolicy {
		mode: "dynamic".into(),
	}));
	let env = prepare_env(&spec).unwrap();
	for e in &env {
		assert!(
			!e.starts_with("HOME="),
			"Dynamic mode should strip HOME, got {e}"
		);
	}
}

#[test]
fn prepare_env_normal_keeps_or_adds_home() {
	let spec = minimal_spec(
		"123e4567-e89b-12d3-a456-426614174001",
		AppExec {
			kind: "command".into(),
			command: Some("echo".into()),
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
	);
	let env = prepare_env(&spec).unwrap();
	let has_home = env.iter().any(|e| e.starts_with("HOME="));
	assert!(has_home, "Normal mode should retain or add HOME");
}
