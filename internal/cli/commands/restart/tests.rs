//! Tests for the `restart` command — 10 cases mirroring
//! `internal/cli/commands/restart/cmd_test.go`.

use std::sync::{Arc, Mutex};

use crate::cli::commands::list::IpcOps as ListIpcOps;
use crate::cli::commands::restart::{run, IpcError, RestartOps, RestartResponse};
use crate::types::{ProcessInfo, ProcessState};

#[derive(Clone, Default)]
struct MockClient {
	list_response: Vec<ProcessInfo>,
	restart_response: Option<RestartResponse>,
	restart_err: Option<String>,
	calls: Arc<Mutex<Vec<(String, String)>>>,
}

impl MockClient {
	fn new() -> Self {
		Self::default()
	}

	fn calls(&self) -> Vec<(String, String)> {
		self.calls.lock().unwrap().clone()
	}
}

impl RestartOps for MockClient {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("list".to_string(), String::new()));
		Ok(self.list_response.clone())
	}

	fn restart(&mut self, id: &str) -> Result<RestartResponse, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("restart".to_string(), id.to_string()));
		if let Some(e) = &self.restart_err {
			return Err(IpcError(e.clone()));
		}
		Ok(self.restart_response.clone().unwrap_or(RestartResponse {
			status: "restarted".into(),
			id: id.to_string(),
		}))
	}
}

impl ListIpcOps for MockClient {
	fn call_list(&mut self) -> Result<Vec<ProcessInfo>, crate::cli::commands::list::IpcError> {
		<MockClient as RestartOps>::list_processes(self)
			.map_err(|e| crate::cli::commands::list::IpcError(e.0))
	}
}

/// Scripted mock for the partial-failure JSON test.
struct ScriptedClient {
	fail_on_second: bool,
	calls: Arc<Mutex<Vec<(String, String)>>>,
}

impl RestartOps for ScriptedClient {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("list".to_string(), String::new()));
		Ok(Vec::new())
	}

	fn restart(&mut self, id: &str) -> Result<RestartResponse, IpcError> {
		self.calls
			.lock()
			.unwrap()
			.push(("restart".to_string(), id.to_string()));
		let count = self
			.calls
			.lock()
			.unwrap()
			.iter()
			.filter(|(cmd, _)| cmd == "restart")
			.count();
		if self.fail_on_second && count == 2 {
			Err(IpcError("not found".into()))
		} else {
			Ok(RestartResponse {
				status: "restarted".into(),
				id: id.to_string(),
			})
		}
	}
}

impl ListIpcOps for ScriptedClient {
	fn call_list(&mut self) -> Result<Vec<ProcessInfo>, crate::cli::commands::list::IpcError> {
		<ScriptedClient as RestartOps>::list_processes(self)
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
fn restart_run_missing_args_errors() {
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
fn restart_run_success_makes_one_call() {
	let mut client = MockClient::new();
	client.restart_response = Some(RestartResponse {
		status: "restarted".into(),
		id: "abc-123".into(),
	});
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["abc-123".to_string(), "--no-list".to_string()];
	run(&mut client, &mut out, &mut err_buf, &args).expect("ok");
	let calls = client.calls();
	assert_eq!(calls.len(), 1);
	assert_eq!(calls[0].0, "restart");
	assert_eq!(calls[0].1, "abc-123");
}

#[test]
fn restart_run_ipc_error_propagates() {
	let mut client = MockClient::new();
	client.restart_err = Some("connection refused".to_string());
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["abc-123".to_string(), "--no-list".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_err(), "expected non-nil error from IPC failure");
}

#[test]
fn restart_run_multiple_ids_makes_one_call_each() {
	let mut client = MockClient::new();
	client.restart_response = Some(RestartResponse {
		status: "restarted".into(),
		id: "x".into(),
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
	let count = calls.iter().filter(|(cmd, _)| cmd == "restart").count();
	assert_eq!(count, 3);
}

#[test]
fn restart_command_spec_has_name() {
	use crate::cli::commands::restart::spec;
	let s = spec();
	assert_eq!(s.name, "restart");
}

#[test]
fn restart_run_json_output_and_partial_failure() {
	let mut client = ScriptedClient {
		fail_on_second: true,
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
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("1 of 3"), "got {msg:?}");
	let json = String::from_utf8_lossy(&out);
	let parsed: serde_json::Value = serde_json::from_str(json.trim()).expect("valid json");
	assert_eq!(parsed["op"], "restart");
	let summary = &parsed["summary"];
	assert_eq!(summary["total"], 3);
	assert_eq!(summary["ok"], 2);
	assert_eq!(summary["failed"], 1);
}

#[test]
fn restart_run_namespace_flag_expands_all_procs_in_ns() {
	let procs = vec![
		proc("id-prod-api", "api", "prod"),
		proc("id-prod-worker", "worker", "prod"),
		proc("id-dev-api", "api", "dev"),
	];
	let mut client = MockClient::new();
	client.list_response = procs;
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["--namespace".to_string(), "prod".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_ok(), "got {result:?}");
	let calls = client.calls();
	let stop_count = calls.iter().filter(|(cmd, _)| cmd == "restart").count();
	assert_eq!(stop_count, 2, "expected 2 restarts for namespace prod");
}

#[test]
fn restart_run_namespace_wildcard_expands_all_procs_in_ns() {
	let procs = vec![
		proc("id-prod-api", "api", "prod"),
		proc("id-prod-worker", "worker", "prod"),
	];
	let mut client = MockClient::new();
	client.list_response = procs;
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["prod:*".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_ok(), "got {result:?}");
	let calls = client.calls();
	let stop_count = calls.iter().filter(|(cmd, _)| cmd == "restart").count();
	assert_eq!(stop_count, 2);
}

#[test]
fn restart_run_namespace_flag_rejects_mix_with_positional() {
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
fn restart_run_empty_namespace_errors() {
	let mut client = MockClient::new();
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["--namespace".to_string(), "ghost".to_string()];
	let result = run(&mut client, &mut out, &mut err_buf, &args);
	assert!(result.is_err(), "expected empty-namespace error");
}
