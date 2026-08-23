//! Tests for [`service`](super). Mirrors `service_test.go`.
//!
//! Seven top-level test functions, all going through
//! [`start_process`](super::start_process) against a freshly-built
//! [`SharedManager`]. Cases that touch the filesystem (`env_file`, `cwd`)
//! need a temp dir; cases that touch `XDG_CONFIG_HOME` (via
//! `mgr.start_with_spec` → `spec.save_spec_protocol`) acquire
//! [`tests::EnvGuard`](super::super::tests::EnvGuard) so a panicking test
//! cannot leak state into the next.

#![cfg(target_os = "linux")]

use std::collections::BTreeMap;

use crate::daemon::handlers::service::start_process;
use crate::ipc::protocol::{AppExec, AppLogs, AppResources, AppSpec, AppStop, RunAsPolicy};
use uuid::Uuid;

use super::super::tests::{new_manager, self_identity, EnvGuard};

fn base_spec() -> AppSpec {
	AppSpec {
		version: 1,
		id: Uuid::now_v7().to_string(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some("echo".into()),
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: None,
		env: None,
		env_file: None,
		logs: None,
		restart: None,
		cron: None,
		run_as: Some(Box::new(RunAsPolicy {
			mode: "self".into(),
		})),
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	}
}

type ExecCase = (&'static str, Box<dyn Fn(&mut AppSpec)>, &'static str);

fn isolate_xdg() -> (EnvGuard, tempfile::TempDir) {
	let guard = EnvGuard::new();
	let temp = tempfile::tempdir().expect("tempdir");
	std::env::set_var("XDG_CONFIG_HOME", temp.path());
	std::env::set_var("XDG_STATE_HOME", temp.path());
	std::env::set_var("HOME", temp.path());
	(guard, temp)
}

#[test]
fn validate_spec_exec_branches() {
	let mgr = new_manager();
	let id = self_identity();
	let cases: Vec<ExecCase> = vec![
		(
			"invalid exec type",
			Box::new(|s| s.exec.kind = "weird".into()),
			"invalid exec type",
		),
		(
			"entry missing",
			Box::new(|s| {
				s.exec = AppExec {
					kind: "entry".into(),
					command: None,
					args: None,
					entry: None,
					runtime: None,
					shell: false,
				}
			}),
			"entry file is required",
		),
		(
			"arg too long",
			Box::new(|s| {
				s.exec.args = Some(vec!["a".repeat(4097)]);
			}),
			"argument too long",
		),
		(
			"env value too long",
			Box::new(|s| {
				let mut m = BTreeMap::new();
				m.insert("k".into(), "v".repeat(8193));
				s.env = Some(m);
			}),
			"env value too long",
		),
		(
			"env key too long",
			Box::new(|s| {
				let mut m = BTreeMap::new();
				m.insert("k".repeat(257), "v".into());
				s.env = Some(m);
			}),
			"env key too long",
		),
		(
			"namespace bad",
			Box::new(|s| s.namespace = Some("bad ns".into())),
			"invalid namespace format",
		),
		(
			"cron too long",
			Box::new(|s| s.cron = Some("a".repeat(257))),
			"cron spec too long",
		),
		(
			"cron newline",
			Box::new(|s| s.cron = Some("* * *\n* *".into())),
			"invalid cron spec",
		),
	];

	for (name, mod_spec, want) in cases {
		let (_env, _temp) = isolate_xdg();
		let mut s = base_spec();
		mod_spec(&mut s);
		let err = start_process(&mgr, s, &id, false).expect_err(&format!("{name}: expected error"));
		assert!(
			err.contains(want),
			"{name}: err={err:?} want substring {want:?}"
		);
	}
}

#[test]
fn validate_spec_logs_branches() {
	let mgr = new_manager();
	let id = self_identity();
	let cases: Vec<(&str, AppLogs, &str)> = vec![
		(
			"bad mode",
			AppLogs {
				mode: "weird".into(),
				dir: None,
				stdout: None,
				stderr: None,
				format: None,
				timestamp: None,
			},
			"invalid logs mode",
		),
		(
			"bad format",
			AppLogs {
				mode: "file".into(),
				dir: None,
				stdout: None,
				stderr: None,
				format: Some("yaml".into()),
				timestamp: None,
			},
			"invalid logs format",
		),
		(
			"bad timestamp",
			AppLogs {
				mode: "file".into(),
				dir: None,
				stdout: None,
				stderr: None,
				format: None,
				timestamp: Some("iso".into()),
			},
			"invalid logs timestamp",
		),
		(
			"dir too long",
			AppLogs {
				mode: "file".into(),
				dir: Some("a".repeat(4097)),
				stdout: None,
				stderr: None,
				format: None,
				timestamp: None,
			},
			"log dir too long",
		),
		(
			"path traversal",
			AppLogs {
				mode: "file".into(),
				dir: Some("../../etc".into()),
				stdout: None,
				stderr: None,
				format: None,
				timestamp: None,
			},
			"must not contain '..'",
		),
		(
			"abs stdout",
			AppLogs {
				mode: "file".into(),
				dir: None,
				stdout: Some("/tmp/x.log".into()),
				stderr: None,
				format: None,
				timestamp: None,
			},
			"logs.stdout must be a relative filename",
		),
		(
			"abs stderr",
			AppLogs {
				mode: "file".into(),
				dir: None,
				stdout: None,
				stderr: Some("/tmp/x.log".into()),
				format: None,
				timestamp: None,
			},
			"logs.stderr must be a relative filename",
		),
	];

	for (name, logs, want) in cases {
		let (_env, _temp) = isolate_xdg();
		let mut s = base_spec();
		s.logs = Some(Box::new(logs));
		let err = start_process(&mgr, s, &id, false).expect_err(&format!("{name}: expected error"));
		assert!(
			err.contains(want),
			"{name}: err={err:?} want substring {want:?}"
		);
	}
}

#[test]
fn validate_spec_stop_branches() {
	let mgr = new_manager();
	let id = self_identity();
	let cases: Vec<(&str, AppStop, &str)> = vec![
		(
			"invalid signal",
			AppStop {
				signal: Some("SIGFAKE".into()),
				timeout_ms: None,
			},
			"invalid stop signal",
		),
		(
			"timeout too small",
			AppStop {
				signal: None,
				timeout_ms: Some(500),
			},
			"stop.timeout_ms",
		),
		(
			"timeout too big",
			AppStop {
				signal: None,
				timeout_ms: Some(999_999),
			},
			"stop.timeout_ms",
		),
	];

	for (name, stop, want) in cases {
		let (_env, _temp) = isolate_xdg();
		let mut s = base_spec();
		s.stop = Some(Box::new(stop));
		let err = start_process(&mgr, s, &id, false).expect_err(&format!("{name}: expected error"));
		assert!(
			err.contains(want),
			"{name}: err={err:?} want substring {want:?}"
		);
	}
}

#[test]
fn validate_spec_resources_branches() {
	let mgr = new_manager();
	let id = self_identity();
	let cases: Vec<(&str, AppResources, &str)> = vec![
		(
			"neg memory",
			AppResources {
				memory_max_bytes: Some(-1),
				cpu_max_percent: None,
				tasks_max: None,
			},
			"memory_max_bytes must be >= 0",
		),
		(
			"tiny memory",
			AppResources {
				memory_max_bytes: Some(1024),
				cpu_max_percent: None,
				tasks_max: None,
			},
			"memory_max_bytes must be >= 1 MiB",
		),
		(
			"neg cpu",
			AppResources {
				memory_max_bytes: None,
				cpu_max_percent: Some(-1),
				tasks_max: None,
			},
			"cpu_max_percent",
		),
		(
			"big cpu",
			AppResources {
				memory_max_bytes: None,
				cpu_max_percent: Some(100_000),
				tasks_max: None,
			},
			"cpu_max_percent",
		),
		(
			"neg tasks",
			AppResources {
				memory_max_bytes: None,
				cpu_max_percent: None,
				tasks_max: Some(-1),
			},
			"tasks_max",
		),
	];

	for (name, res, want) in cases {
		let (_env, _temp) = isolate_xdg();
		let mut s = base_spec();
		s.resources = Some(Box::new(res));
		let err = start_process(&mgr, s, &id, false).expect_err(&format!("{name}: expected error"));
		assert!(
			err.contains(want),
			"{name}: err={err:?} want substring {want:?}"
		);
	}
}

#[test]
fn validate_env_file_via_start() {
	let mgr = new_manager();
	let id = self_identity();
	let tmp = tempfile::tempdir().expect("tempdir");
	let env_path = tmp.path().join("env");
	std::fs::write(&env_path, b"FOO=bar\n").expect("write env");

	{
		let (_env, _temp) = isolate_xdg();
		let mut too_long = base_spec();
		too_long.env_file = Some("/a".repeat(2200));
		let err = start_process(&mgr, too_long, &id, false).expect_err("too long");
		assert!(err.contains("env_file path too long"), "got {err:?}");
	}

	{
		let (_env, _temp) = isolate_xdg();
		let mut dot_dot = base_spec();
		dot_dot.env_file = Some("../foo".into());
		let err = start_process(&mgr, dot_dot, &id, false).expect_err("dot-dot");
		assert!(err.contains("must not contain '..'"), "got {err:?}");
	}

	{
		let (_env, _temp) = isolate_xdg();
		let mut not_reg = base_spec();
		not_reg.env_file = Some(tmp.path().to_string_lossy().into_owned());
		let err = start_process(&mgr, not_reg, &id, false).expect_err("not regular");
		assert!(err.contains("regular file"), "got {err:?}");
	}

	{
		let (_env, _temp) = isolate_xdg();
		let mut not_acc = base_spec();
		not_acc.env_file = Some(tmp.path().join("missing").to_string_lossy().into_owned());
		let err = start_process(&mgr, not_acc, &id, false).expect_err("not accessible");
		assert!(err.contains("not accessible"), "got {err:?}");
	}

	#[cfg(unix)]
	{
		use std::os::unix::fs::MetadataExt;
		let (_env, _temp) = isolate_xdg();
		let meta = std::fs::metadata(&env_path).expect("stat env");
		let other_uid = meta.uid() + 1;
		let mut not_owner = base_spec();
		not_owner.env_file = Some(env_path.to_string_lossy().into_owned());
		let foreign = crate::ipc::transport::Identity {
			uid: other_uid.to_string(),
			gid: std::env::var("GID").unwrap_or_else(|_| "1000".into()),
			pid: std::process::id() as i32,
		};
		let err = start_process(&mgr, not_owner, &foreign, false).expect_err("not owned");
		assert!(err.contains("not owned by caller"), "got {err:?}");
	}

	// Relative env_file skips the ownership check entirely (no I/O).
	{
		let (_env, _temp) = isolate_xdg();
		let mut relative = base_spec();
		relative.env_file = Some("rel/env".into());
		if let Err(e) = start_process(&mgr, relative, &id, false) {
			assert!(
				!e.contains("env_file"),
				"relative env_file should be allowed, got {e:?}"
			);
		}
	}
}

#[test]
fn start_process_cwd_restricted() {
	let (_env, _temp) = isolate_xdg();
	let mgr = new_manager();
	let id = self_identity();
	let mut s = base_spec();
	s.cwd = Some("/etc".into());
	let err = start_process(&mgr, s, &id, false).expect_err("restricted");
	assert!(err.contains("restricted system directory"), "got {err:?}");
}

#[test]
fn start_process_cwd_too_long() {
	let (_env, _temp) = isolate_xdg();
	let mgr = new_manager();
	let id = self_identity();
	let mut s = base_spec();
	s.cwd = Some("a".repeat(4097));
	let err = start_process(&mgr, s, &id, false).expect_err("too long");
	assert!(err.contains("cwd too long"), "got {err:?}");
}
