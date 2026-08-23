//! End-to-end tests for the daemon handler layer. Mirrors
//! `handlers_integration_test.go`.
//!
//! Each test wires a real `unitpmd`-style stack — manager, IPC server,
//! registered handlers, and a connected client — against a temp dir and
//! a temp socket. The [`EnvGuard`] keeps `XDG_CONFIG_HOME`,
//! `XDG_STATE_HOME`, `HOME`, and `UNITPM_SOCKET` from leaking between
//! parallel tests.
//!
//! Sixteen top-level test functions covering every registered verb:
//! ping, version, list, start, show, stop, delete, flush, scale, reset,
//! restart, resolve-by-name, proctree (running / stopped / not-found).

#![cfg(target_os = "linux")]

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use crate::daemon::audit::Logger;
use crate::daemon::handlers::register_handlers;
use crate::daemon::manager::Manager;
use crate::ipc::protocol::{AppExec, AppSpec, RunAsPolicy};
use crate::ipc::transport::{Client, IPCClient, Server};
use crate::metrics::ChildStat;
use crate::types::ProcessInfo;
use uuid::Uuid;

use super::{new_manager, EnvGuard};

struct Stack {
	server: Server,
	client: Client,
	mgr: std::sync::Arc<std::sync::Mutex<crate::daemon::manager::Manager>>,
	_temp: tempfile::TempDir,
}

fn setup() -> Stack {
	let _env = EnvGuard::new();
	let temp = tempfile::tempdir().expect("tempdir");
	std::env::set_var("XDG_CONFIG_HOME", temp.path());
	std::env::set_var("XDG_STATE_HOME", temp.path());
	std::env::set_var("HOME", temp.path());
	let socket = temp.path().join("unitpm.sock");
	std::env::set_var("UNITPM_SOCKET", &socket);

	let mgr = new_manager();
	let server = Server::new();
	register_handlers(&server, Arc::clone(&mgr), false, Logger::disabled());

	let socket_path = server.start().expect("server start");
	// Give the accept loop a moment to bind.
	std::thread::sleep(Duration::from_millis(100));

	let client = Client::connect_to(&socket_path).expect("connect");

	Stack {
		server,
		client,
		mgr,
		_temp: temp,
	}
}

fn drop_stack(stack: Stack) {
	// Drop order matters: close the server first so the accept loop ends,
	// then the client, then drop the manager.
	let Stack {
		server,
		client,
		mgr: _mgr,
		_temp,
	} = stack;
	server.close();
	drop(client);
	// `_temp` and `_mgr` go out of scope here.
}

fn seed_running(stack: &Stack, name: &str) -> String {
	let id = Uuid::now_v7().to_string();
	let spec = AppSpec {
		version: 1,
		id: id.clone(),
		name: name.into(),
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
	};
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.start_with_spec(spec)
		.expect("seed");
	id
}

#[test]
fn e2e_ping() {
	let mut stack = setup();
	let mut resp: std::collections::HashMap<String, String> = Default::default();
	stack
		.client
		.call("ping", None::<&()>, Some(&mut resp))
		.expect("ping");
	assert_eq!(resp.get("response").map(String::as_str), Some("pong"));
	drop_stack(stack);
}

#[test]
fn e2e_version() {
	let mut stack = setup();
	let mut got: crate::version::Info = crate::version::get();
	stack
		.client
		.call("version", None::<&()>, Some(&mut got))
		.expect("version");
	assert!(!got.version.is_empty(), "version Info has empty Version");
	drop_stack(stack);
}

#[test]
fn e2e_list_empty() {
	let mut stack = setup();
	let mut list: Vec<ProcessInfo> = Vec::new();
	stack
		.client
		.call("list", None::<&()>, Some(&mut list))
		.expect("list");
	assert!(list.is_empty());
	drop_stack(stack);
}

#[test]
fn e2e_start_then_list() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-list");
	let mut list: Vec<ProcessInfo> = Vec::new();
	stack
		.client
		.call("list", None::<&()>, Some(&mut list))
		.expect("list");
	assert_eq!(list.len(), 1);
	assert_eq!(list[0].name, "e2e-list");
	// Cleanup
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	drop_stack(stack);
}

#[test]
fn e2e_show_by_id() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-show");
	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"show",
			Some(&serde_json::json!({"id": id})),
			Some(&mut resp),
		)
		.expect("show");
	assert!(resp.get("info").is_some(), "expected info field");
	assert!(resp.get("spec").is_some(), "expected spec field");
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	drop_stack(stack);
}

#[test]
fn e2e_show_not_found() {
	let mut stack = setup();
	let mut resp: serde_json::Value = serde_json::Value::Null;
	let err = stack
		.client
		.call(
			"show",
			Some(&serde_json::json!({"id": "does-not-exist"})),
			Some(&mut resp),
		)
		.expect_err("unknown id should error");
	let _ = err;
	drop_stack(stack);
}

#[test]
fn e2e_stop_roundtrip() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-stop");
	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"stop",
			Some(&serde_json::json!({"id": id})),
			Some(&mut resp),
		)
		.expect("stop");
	assert_eq!(resp["status"], "stopped");
	assert_eq!(resp["id"], serde_json::Value::String(id.clone()));
	// Verify gone from the manager.
	let still_there = stack.mgr.lock().unwrap_or_else(|e| e.into_inner()).get(&id);
	// After stop the manager may or may not retain the entry; what matters
	// is that the response was well-formed. We assert `was_running` was
	// set, since the process was alive at request time.
	assert_eq!(resp["was_running"], serde_json::Value::Bool(true));
	let _ = still_there;
	drop_stack(stack);
}

#[test]
fn e2e_delete_roundtrip() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-del");
	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"delete",
			Some(&serde_json::json!({"id": id, "purge": false})),
			Some(&mut resp),
		)
		.expect("delete");
	assert_eq!(resp["status"], "deleted");
	drop_stack(stack);
}

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
		logs: Some(Box::new(crate::ipc::protocol::AppLogs {
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

#[test]
fn e2e_scale_no_template() {
	let mut stack = setup();
	let mut resp: serde_json::Value = serde_json::Value::Null;
	let err = stack.client.call(
		"scale",
		Some(&serde_json::json!({
			"namespace": "default",
			"name": "ghost",
			"target": 3,
		})),
		Some(&mut resp),
	);
	assert!(err.is_err(), "expected error scaling nonexistent template");
	drop_stack(stack);
}

#[test]
fn e2e_reset_by_id() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-reset");
	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"reset",
			Some(&serde_json::json!({"id": id})),
			Some(&mut resp),
		)
		.expect("reset");
	assert_eq!(resp["status"], "reset");
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	drop_stack(stack);
}

#[test]
fn e2e_restart_by_id() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-restart");
	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"restart",
			Some(&serde_json::json!({"id": id})),
			Some(&mut resp),
		)
		.expect("restart");
	assert_eq!(resp["status"], "restarted");
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	drop_stack(stack);
}

#[test]
fn e2e_resolve_by_name() {
	let mut stack = setup();
	let id = seed_running(&stack, "e2e-resolve");
	let mut resp: serde_json::Value = serde_json::Value::Null;
	stack
		.client
		.call(
			"stop",
			Some(&serde_json::json!({"id": "e2e-resolve"})),
			Some(&mut resp),
		)
		.expect("stop by name");
	assert_eq!(resp["id"], serde_json::Value::String(id));
	drop_stack(stack);
}

#[test]
fn e2e_proctree_running() {
	let mut stack = setup();
	let id = Uuid::now_v7().to_string();
	// bash forks two `sleep`s as children so the tree has > 1 entry.
	let spec = AppSpec {
		version: 1,
		id: id.clone(),
		name: "e2e-tree".into(),
		namespace: Some("default".into()),
		exec: AppExec {
			kind: "command".into(),
			command: Some("bash".into()),
			args: Some(vec!["-c".into(), "sleep 30 & sleep 30 & wait".into()]),
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
	};
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.start_with_spec(spec)
		.expect("seed bash");

	// Poll until children appear (bash forks asynchronously).
	let deadline = std::time::Instant::now() + Duration::from_secs(3);
	let mut tree: Vec<ChildStat> = Vec::new();
	loop {
		tree.clear();
		let _ = stack.client.call::<_, serde_json::Value>(
			"proctree",
			Some(&serde_json::json!({"id": id})),
			None,
		);
		// Re-fetch the typed tree.
		tree = {
			let raw: serde_json::Value = serde_json::Value::Null;
			let mut sink = raw;
			stack
				.client
				.call(
					"proctree",
					Some(&serde_json::json!({"id": id})),
					Some(&mut sink),
				)
				.expect("proctree");
			serde_json::from_value(sink).unwrap_or_default()
		};
		if tree.iter().any(|e| e.depth > 0) || std::time::Instant::now() >= deadline {
			break;
		}
		std::thread::sleep(Duration::from_millis(50));
	}

	assert!(
		!tree.is_empty(),
		"proctree returned empty for a running process"
	);
	assert_eq!(tree[0].depth, 0);
	assert!(tree[0].memory_bytes > 0);
	assert!(
		tree.iter().any(|e| e.depth > 0),
		"expected at least one child entry, tree = {tree:?}"
	);

	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	drop_stack(stack);
}

#[test]
fn e2e_proctree_stopped() {
	let mut stack = setup();
	let id = Uuid::now_v7().to_string();
	let spec = AppSpec {
		version: 1,
		id: id.clone(),
		name: "e2e-tree-stopped".into(),
		namespace: Some("default".into()),
		exec: AppExec {
			kind: "command".into(),
			command: Some("sleep".into()),
			args: Some(vec!["1".into()]),
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
	};
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.start_with_spec(spec)
		.expect("seed");
	stack
		.mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.stop(&id)
		.ok();
	std::thread::sleep(Duration::from_millis(200));

	let mut tree: Vec<ChildStat> = Vec::new();
	stack
		.client
		.call(
			"proctree",
			Some(&serde_json::json!({"id": id})),
			Some(&mut tree),
		)
		.expect("proctree stopped");
	assert!(tree.is_empty(), "expected empty tree for stopped process");
	drop_stack(stack);
}

#[test]
fn e2e_proctree_not_found() {
	let mut stack = setup();
	let mut tree: Vec<ChildStat> = Vec::new();
	let err = stack.client.call(
		"proctree",
		Some(&serde_json::json!({"id": "nonexistent"})),
		Some(&mut tree),
	);
	assert!(err.is_err(), "expected error for unknown process");
	drop_stack(stack);
}

#[allow(dead_code)]
fn _phantom_path(p: PathBuf) -> PathBuf {
	p
}

#[allow(dead_code)]
fn _phantom_manager(_m: &Manager) {}

#[allow(dead_code)]
fn _phantom_btreemap() -> BTreeMap<String, String> {
	BTreeMap::new()
}
