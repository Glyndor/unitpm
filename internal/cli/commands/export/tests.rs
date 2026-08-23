//! Tests for the export command.
//!
//! 10 cases ported from `internal/cli/commands/export/cmd_test.go`.

use std::path::PathBuf;

use crate::cli::commands::export;
use crate::ipc::protocol::{AppExec, AppLogs, AppRestart, AppSpec};
use crate::spec;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

/// Pin the XDG_CONFIG_HOME to a temp dir so spec saves / loads land in a
/// sandbox. Restores the previous value on Drop.
struct XdgGuard {
	prev: Option<String>,
}

impl XdgGuard {
	fn new(t: &tempfile::TempDir) -> Self {
		let prev = std::env::var("XDG_CONFIG_HOME").ok();
		std::env::set_var("XDG_CONFIG_HOME", t.path());
		Self { prev }
	}
}

impl Drop for XdgGuard {
	fn drop(&mut self) {
		match &self.prev {
			Some(v) => std::env::set_var("XDG_CONFIG_HOME", v),
			None => std::env::remove_var("XDG_CONFIG_HOME"),
		}
	}
}

#[allow(dead_code)]
fn write_spec(dir: &std::path::Path, id: &str, content: &str) {
	let apps_dir: PathBuf = dir.join("unitpm").join("apps");
	std::fs::create_dir_all(&apps_dir).expect("mkdir");
	let path = apps_dir.join(format!("{id}.json"));
	std::fs::write(&path, content).expect("write");
}

fn app_spec_command(name: &str, namespace: &str) -> AppSpec {
	AppSpec {
		version: 1,
		id: format!("id-{name}"),
		name: name.into(),
		namespace: Some(namespace.into()),
		exec: AppExec {
			kind: "command".into(),
			command: Some("node".into()),
			args: Some(vec!["server.js".into()]),
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: Some("/tmp".into()),
		env: None,
		env_file: None,
		logs: Some(Box::new(AppLogs {
			mode: "file".into(),
			dir: Some("/var/log/glyndor/unitpm".into()),
			stdout: None,
			stderr: None,
			format: None,
			timestamp: None,
		})),
		restart: Some(Box::new(AppRestart {
			policy: "always".into(),
			max_retries: Some(5),
			backoff_ms: Some(1000),
			backoff_type: Some("expo".into()),
			stop_on_exit: Some(vec![0]),
		})),
		cron: None,
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	}
}

fn app_spec_entry(name: &str, namespace: &str) -> AppSpec {
	AppSpec {
		version: 1,
		id: format!("id-{name}"),
		name: name.into(),
		namespace: Some(namespace.into()),
		exec: AppExec {
			kind: "entry".into(),
			command: None,
			args: None,
			entry: Some("index.js".into()),
			runtime: Some("node".into()),
			shell: false,
		},
		cwd: Some("/tmp".into()),
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
fn run_missing_namespace_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = export::run(&mut buf, &[]);
	let err = rc.expect_err("missing args");
	assert!(
		err.to_string().contains("export requires --namespace"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_namespace_no_value_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = export::run(&mut buf, &["--namespace".to_string()]);
	assert!(rc.is_err(), "expected error for --namespace without value");
}

#[test]
fn run_empty_namespace_string_errors() {
	let _g = lock_term();
	let tmp = tempfile::tempdir().expect("tempdir");
	let _xdg = XdgGuard::new(&tmp);
	let mut buf = Vec::new();
	let rc = export::run(&mut buf, &["--namespace".to_string(), String::new()]);
	let err = rc.expect_err("empty namespace");
	assert!(
		err.to_string().contains("missing --namespace"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	export::run(&mut buf, &["--help".to_string()]).expect("ok");
}

#[test]
fn run_no_apps_in_namespace_errors() {
	let _g = lock_term();
	let tmp = tempfile::tempdir().expect("tempdir");
	let _xdg = XdgGuard::new(&tmp);
	let mut buf = Vec::new();
	let rc = export::run(&mut buf, &["--namespace".to_string(), "prod".to_string()]);
	let err = rc.expect_err("empty namespace");
	assert!(
		err.to_string().contains("no apps found"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_success_command_kind() {
	let _g = lock_term();
	let tmp = tempfile::tempdir().expect("tempdir");
	let _xdg = XdgGuard::new(&tmp);
	let s = app_spec_command("api", "prod");
	spec::save_spec_protocol(&s.id, &s).expect("save");

	let mut buf = Vec::new();
	export::run(&mut buf, &["--namespace".to_string(), "prod".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(out.contains("api"), "expected name in output: {out:?}");
	assert!(out.contains("node"), "expected command in output: {out:?}");
}

#[test]
fn run_success_entry_kind() {
	let _g = lock_term();
	let tmp = tempfile::tempdir().expect("tempdir");
	let _xdg = XdgGuard::new(&tmp);
	let s = app_spec_entry("worker", "prod");
	spec::save_spec_protocol(&s.id, &s).expect("save");

	let mut buf = Vec::new();
	export::run(&mut buf, &["--namespace".to_string(), "prod".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(out.contains("worker"), "expected name in output: {out:?}");
	assert!(
		out.contains("index.js"),
		"expected entry path in output: {out:?}"
	);
}

#[test]
fn run_filters_by_namespace() {
	let _g = lock_term();
	let tmp = tempfile::tempdir().expect("tempdir");
	let _xdg = XdgGuard::new(&tmp);

	let api = app_spec_command("api", "prod");
	spec::save_spec_protocol(&api.id, &api).expect("save");
	let dev = app_spec_command("dev", "staging");
	spec::save_spec_protocol(&dev.id, &dev).expect("save");

	let mut buf = Vec::new();
	export::run(&mut buf, &["--namespace".to_string(), "prod".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(out.contains("api"), "prod should include api");
	assert!(!out.contains("dev"), "prod should exclude dev");

	let mut buf = Vec::new();
	export::run(
		&mut buf,
		&["--namespace".to_string(), "staging".to_string()],
	)
	.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(!out.contains("api"), "staging should exclude api");
	assert!(out.contains("dev"), "staging should include dev");

	let mut buf = Vec::new();
	let rc = export::run(
		&mut buf,
		&["--namespace".to_string(), "nonexistent".to_string()],
	);
	assert!(rc.is_err(), "nonexistent namespace must error");
}

#[test]
fn run_short_flag_works() {
	let _g = lock_term();
	let tmp = tempfile::tempdir().expect("tempdir");
	let _xdg = XdgGuard::new(&tmp);
	let s = app_spec_command("api", "prod");
	spec::save_spec_protocol(&s.id, &s).expect("save");

	let mut buf = Vec::new();
	export::run(&mut buf, &["-n".to_string(), "prod".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(out.contains("api"));
}

#[test]
fn get_spec_matches_name() {
	let s = export::spec();
	assert_eq!(s.name, "export");
}
