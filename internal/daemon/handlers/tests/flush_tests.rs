//! End-to-end tests for the `flush` verb.
//!
//! Mirrors the corresponding test in
//! `internal/daemon/handlers/handlers_integration_test.go`.
//! Specifically validates that the daemon's reported `bytes_freed`
//! matches what was on disk before the call and that the files are
//! actually truncated afterwards.
//!
//! The setup writes pre-populated stdout/stderr files at temp paths
//! referenced from the seeded spec's `logs.dir / stdout / stderr`,
//! starts the spec, then issues the IPC `flush` call and asserts on
//! the response shape plus the resulting filesystem state.

#![cfg(target_os = "linux")]

use uuid::Uuid;

use crate::daemon::handlers::tests::stack::{drop_stack, setup};
use crate::ipc::protocol::{AppExec, AppLogs, AppSpec, RunAsPolicy};
use crate::ipc::transport::IPCClient;

#[test]
fn e2e_flush_bytes_freed() {
	let mut stack = setup();

	// Seed a process whose spec points Logs.Dir / Stdout / Stderr at a
	// temp directory we pre-populate so `flush` has bytes to free.
	let id = Uuid::now_v7().to_string();
	let log_dir = std::env::temp_dir().join(format!("flush-{id}"));
	let _ = std::fs::create_dir_all(&log_dir);
	let stdout_path = log_dir.join("stdout.log");
	let stderr_path = log_dir.join("stderr.log");
	std::fs::write(&stdout_path, b"hello stdout\n").expect("write stdout");
	std::fs::write(&stderr_path, b"hello stderr\n").expect("write stderr");

	let spec = AppSpec {
		version: 1,
		id: id.clone(),
		name: "e2e-flush".into(),
		namespace: Some("default".into()),
		exec: AppExec {
			kind: "command".into(),
			command: Some("sleep".into()),
			args: Some(vec!["30".into()]),
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: None,
		env: None,
		env_file: None,
		logs: Some(Box::new(AppLogs {
			mode: "file".into(),
			dir: Some(log_dir.to_string_lossy().into_owned()),
			stdout: Some(stdout_path.to_string_lossy().into_owned()),
			stderr: Some(stderr_path.to_string_lossy().into_owned()),
			format: None,
			timestamp: None,
		})),
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
	};
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.start_with_spec(spec)
		.expect("seed");

	// Read actual on-disk sizes after Start (which appends a STARTED banner)
	// so the assertion is robust to banner length changes.
	let si_out = std::fs::metadata(&stdout_path).expect("stat stdout pre-flush");
	let si_err = std::fs::metadata(&stderr_path).expect("stat stderr pre-flush");
	let before = si_out.len() + si_err.len();

	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"flush",
			Some(&serde_json::json!({"id": id})),
			Some(&mut resp),
		)
		.expect("flush");
	assert_eq!(resp["status"], "flushed");
	let got = resp["bytes_freed"].as_i64().unwrap_or(-1);
	assert_eq!(got, before as i64, "bytes_freed mismatch");

	// Files should be truncated on disk.
	for p in [&stdout_path, &stderr_path] {
		let info = std::fs::metadata(p).expect("stat");
		assert_eq!(
			info.len(),
			0,
			"expected {} truncated, size={}",
			p.display(),
			info.len()
		);
	}

	// Cleanup
	let _ = std::fs::remove_dir_all(&log_dir);
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	drop_stack(stack);
}
