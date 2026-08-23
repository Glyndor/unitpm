//! Tests for the `stop` command — 12 cases mirroring
//! `internal/cli/commands/stop/cmd_test.go`.

use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};

use crate::cli::commands::list::IpcOps as ListIpcOps;
use crate::cli::commands::stop::{run, IpcError, StopOps, StopResponse};
use crate::types::{ProcessInfo, ProcessState};

// --- Mock IPC client --------------------------------------------------------

#[derive(Clone, Default)]
struct MockClient {
	list_response: Vec<ProcessInfo>,
	stop_response: Option<StopResponse>,
	stop_err: Option<String>,
	calls: Arc<Mutex<Vec<(String, String)>>>,
	call_list_count: Arc<AtomicU32>,
}

impl MockClient {
	fn new() -> Self {
		Self::default()
	}

	fn calls(&self) -> Vec<(String, String)> {
		self.calls.lock().unwrap().clone()
	}
}

impl StopOps for MockClient {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("list".to_string(), String::new()));
		self.call_list_count.fetch_add(1, Ordering::Relaxed);
		Ok(self.list_response.clone())
	}

	fn stop(&mut self, id: &str) -> Result<StopResponse, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("stop".to_string(), id.to_string()));
		if let Some(e) = &self.stop_err {
			return Err(IpcError(e.clone()));
		}
		Ok(self.stop_response.clone().unwrap_or(StopResponse {
			status: "stopped".into(),
			id: id.to_string(),
			was_running: true,
		}))
	}
}

impl ListIpcOps for MockClient {
	fn call_list(&mut self) -> Result<Vec<ProcessInfo>, crate::cli::commands::list::IpcError> {
		<MockClient as StopOps>::list_processes(self)
			.map_err(|e| crate::cli::commands::list::IpcError(e.0))
	}
}

/// ScriptedClient lets each (cmd, target) produce a different response.
/// Used for cases that exercise the partial-failure / per-call branching.
struct ScriptedClient {
	pub script: ScriptKind,
	pub calls: Arc<Mutex<Vec<(String, String)>>>,
}

enum ScriptKind {
	/// Stop returns Ok with was_running=true for call N=1,3,5,...
	/// noop with was_running=false for call N=2,4,6,...
	/// error for the rest. List returns the configured vec.
	OkNoopFail(Vec<ProcessInfo>),
	/// Stop returns Ok always (was_running=true); call N=2 errors.
	StopSecondFails(Vec<ProcessInfo>),
	/// Stop returns Ok always; list returns configured vec.
	StopOk(Vec<ProcessInfo>),
}

impl StopOps for ScriptedClient {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("list".to_string(), String::new()));
		match &self.script {
			ScriptKind::OkNoopFail(v) | ScriptKind::StopSecondFails(v) | ScriptKind::StopOk(v) => {
				Ok(v.clone())
			}
		}
	}

	fn stop(&mut self, id: &str) -> Result<StopResponse, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("stop".to_string(), id.to_string()));
		match &mut self.script {
			ScriptKind::OkNoopFail(_) => {
				let count = self
					.calls
					.lock()
					.unwrap()
					.iter()
					.filter(|(cmd, _)| cmd == "stop")
					.count();
				match count {
					1 => Ok(StopResponse {
						status: "stopped".into(),
						id: id.to_string(),
						was_running: true,
					}),
					2 => Ok(StopResponse {
						status: "stopped".into(),
						id: id.to_string(),
						was_running: false,
					}),
					_ => Err(IpcError("not found".into())),
				}
			}
			ScriptKind::StopSecondFails(_) => {
				let count = self
					.calls
					.lock()
					.unwrap()
					.iter()
					.filter(|(cmd, _)| cmd == "stop")
					.count();
				if count == 2 {
					Err(IpcError("boom".into()))
				} else {
					Ok(StopResponse {
						status: "stopped".into(),
						id: id.to_string(),
						was_running: true,
					})
				}
			}
			ScriptKind::StopOk(_) => Ok(StopResponse {
				status: "stopped".into(),
				id: id.to_string(),
				was_running: true,
			}),
		}
	}
}

impl ListIpcOps for ScriptedClient {
	fn call_list(&mut self) -> Result<Vec<ProcessInfo>, crate::cli::commands::list::IpcError> {
		<ScriptedClient as StopOps>::list_processes(self)
			.map_err(|e| crate::cli::commands::list::IpcError(e.0))
	}
}

fn proc(id: &str, name: &str, namespace: &str) -> ProcessInfo {
	ProcessInfo {
		id: id.into(),
		name: name.into(),
		namespace: namespace.into(),
		version: String::new(),
		mode: "fork".into(),
		pid: 0,
		uptime: 0,
		restarts: 0,
		state: ProcessState::Running,
		cpu: 0.0,
		memory: 0,
		user: String::new(),
		watch: false,
		git_branch: None,
		git_commit: None,
		git_dirty: false,
		created_at: None,
	}
}

#[test]
fn stop_run_missing_args_errors() {
	let mut client = MockClient::new();
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args: Vec<String> = Vec::new();
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("missing process ID or name"), "got {msg:?}");
}

#[test]
fn stop_run_success_was_running_makes_one_call() {
	let mut client = MockClient::new();
	client.stop_response = Some(StopResponse {
		status: "stopped".into(),
		id: "abc-123".into(),
		was_running: true,
	});
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["abc-123".to_string(), "--no-list".to_string()];
	run(&mut client, &mut out, &mut err_buf, &args).expect("ok");
	let calls = client.calls();
	assert_eq!(calls.len(), 1);
	assert_eq!(calls[0].0, "stop");
	assert_eq!(calls[0].1, "abc-123");
}

#[test]
fn stop_run_already_stopped_does_not_error() {
	let mut client = MockClient::new();
	client.stop_response = Some(StopResponse {
		status: "stopped".into(),
		id: "abc-123".into(),
		was_running: false,
	});
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["abc-123".to_string(), "--no-list".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_ok(), "got {result:?}");
}

#[test]
fn stop_run_ipc_error_propagates() {
	let mut client = MockClient::new();
	client.stop_err = Some("connection refused".to_string());
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["abc-123".to_string(), "--no-list".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_err(), "expected non-nil error from IPC failure");
}

#[test]
fn stop_run_multiple_targets_makes_one_call_each() {
	let mut client = MockClient::new();
	client.stop_response = Some(StopResponse {
		status: "stopped".into(),
		id: "x".into(),
		was_running: true,
	});
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"a".to_string(),
		"b".to_string(),
		"c".to_string(),
		"--no-list".to_string(),
	];
	run(&mut client, &mut out, &mut err_buf, &args).expect("ok");
	let calls = client.calls();
	let stop_count = calls.iter().filter(|(cmd, _)| cmd == "stop").count();
	assert_eq!(stop_count, 3);
}

#[test]
fn stop_command_spec_has_name_and_description() {
	use crate::cli::commands::stop::spec;
	let s = spec();
	assert_eq!(s.name, "stop");
	assert!(!s.description.is_empty());
}

#[test]
fn stop_run_json_output_emits_machine_readable_report() {
	let mut client = ScriptedClient {
		script: ScriptKind::OkNoopFail(Vec::new()),
		calls: Arc::new(Mutex::new(Vec::new())),
	};
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"a".to_string(),
		"b".to_string(),
		"c".to_string(),
		"--json".to_string(),
	];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(
		result.is_err(),
		"expected non-nil error from 1 failed target"
	);
	let json = String::from_utf8_lossy(&out);
	let parsed: serde_json::Value = serde_json::from_str(json.trim()).expect("valid json");
	assert_eq!(parsed["op"], "stop");
	let summary = &parsed["summary"];
	assert_eq!(summary["total"], 3);
	assert_eq!(summary["ok"], 1);
	assert_eq!(summary["noop"], 1);
	assert_eq!(summary["failed"], 1);
}

#[test]
fn stop_run_partial_failure_returns_aggregate_error() {
	let mut client = ScriptedClient {
		script: ScriptKind::StopSecondFails(Vec::new()),
		calls: Arc::new(Mutex::new(Vec::new())),
	};
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"a".to_string(),
		"b".to_string(),
		"c".to_string(),
		"--no-list".to_string(),
	];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("1 of 3"), "got {msg:?}");
}

#[test]
fn stop_run_namespace_flag_expands_all_procs_in_ns() {
	let procs = vec![
		proc("id-prod-api", "api", "prod"),
		proc("id-prod-worker", "worker", "prod"),
		proc("id-dev-api", "api", "dev"),
	];
	let mut client = ScriptedClient {
		script: ScriptKind::StopOk(procs),
		calls: Arc::new(Mutex::new(Vec::new())),
	};
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["--namespace".to_string(), "prod".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_ok(), "got {result:?}");
	let calls = client.calls.lock().unwrap().clone();
	let stop_count = calls.iter().filter(|(cmd, _)| cmd == "stop").count();
	assert_eq!(stop_count, 2, "expected 2 stops for namespace prod");
}

#[test]
fn stop_run_namespace_wildcard_expands_all_procs_in_ns() {
	let procs = vec![
		proc("id-prod-api", "api", "prod"),
		proc("id-prod-worker", "worker", "prod"),
		proc("id-dev-api", "api", "dev"),
	];
	let mut client = ScriptedClient {
		script: ScriptKind::StopOk(procs),
		calls: Arc::new(Mutex::new(Vec::new())),
	};
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["prod:*".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_ok(), "got {result:?}");
	let calls = client.calls.lock().unwrap().clone();
	let stop_count = calls.iter().filter(|(cmd, _)| cmd == "stop").count();
	assert_eq!(stop_count, 2, "expected 2 stops via wildcard");
}

#[test]
fn stop_run_namespace_flag_rejects_mix_with_positional() {
	let mut client = MockClient::new();
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"api".to_string(),
		"--namespace".to_string(),
		"prod".to_string(),
	];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("cannot combine --namespace"), "got {msg:?}");
}

#[test]
fn stop_run_empty_namespace_errors_with_namespace_quoted() {
	let mut client = ScriptedClient {
		script: ScriptKind::StopOk(Vec::new()),
		calls: Arc::new(Mutex::new(Vec::new())),
	};
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["--namespace".to_string(), "ghost".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("\"ghost\""), "got {msg:?}");
}
