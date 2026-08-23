//! Tests for [`crate::daemon::manager::process`]. Mirrors the Go-side
//! `TestNewProcess_CronScheduler`, `TestProcess_Tree_NotRunning`, and the
//! `TestCronEveryIntervalBounds` family.

use super::*;
use crate::ipc::protocol::AppExec;

#[test]
fn process_new_rejects_invalid_uuid() {
	let spec = AppSpec {
		version: 1,
		id: "x".into(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some("true".into()),
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
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	};
	let r = Process::new("not-a-uuid", spec);
	assert!(r.is_err());
}

#[test]
fn process_new_accepts_uuid() {
	let id = Uuid::now_v7().to_string();
	let spec = AppSpec {
		version: 1,
		id: id.clone(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some("true".into()),
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
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	};
	let p = Process::new(&id, spec).unwrap();
	assert_eq!(p.info.state, ProcessState::Stopped);
}

#[test]
fn cron_interval_bounds() {
	let id = Uuid::now_v7().to_string();
	let too_fast = AppSpec {
		version: 1,
		id: id.clone(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some("true".into()),
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
		cron: Some("@every 1s".into()),
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	};
	let r = Process::new(&id, too_fast);
	assert!(r.is_err(), "1s should be rejected");

	let id2 = Uuid::now_v7().to_string();
	let too_slow = AppSpec {
		version: 1,
		id: id2.clone(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some("true".into()),
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
		cron: Some("@every 48h".into()),
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	};
	let r = Process::new(&id2, too_slow);
	assert!(r.is_err(), "48h should be rejected");

	let id3 = Uuid::now_v7().to_string();
	let ok = AppSpec {
		version: 1,
		id: id3.clone(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some("true".into()),
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
		cron: Some("@every 10s".into()),
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	};
	let r = Process::new(&id3, ok);
	assert!(r.is_ok(), "@every 10s should be accepted");
}
