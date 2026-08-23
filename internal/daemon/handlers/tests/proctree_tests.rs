//! End-to-end tests for the `proctree` verb.
//!
//! Mirrors the three test cases at the bottom of
//! `internal/daemon/handlers/handlers_integration_test.go`:
//!
//! - `e2e_proctree_running` — bash forks two `sleep`s so the tree has
//!   > 1 entry; we poll until the children show up.
//! - `e2e_proctree_stopped` — process is started, immediately stopped,
//!   and the tree comes back empty.
//! - `e2e_proctree_not_found` — unknown id returns an error.
//!
//! The children sometimes take a beat to materialise after `bash`
//! forks them; the running test polls for a short window instead of
//! asserting on a fixed count.

#![cfg(target_os = "linux")]

use std::time::Duration;

use uuid::Uuid;

use crate::daemon::handlers::tests::stack::{drop_stack, setup};
use crate::ipc::protocol::{AppExec, AppSpec, RunAsPolicy};
use crate::ipc::transport::IPCClient;
use crate::metrics::ChildStat;

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
