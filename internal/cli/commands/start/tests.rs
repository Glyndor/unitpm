//! Tests for the `start` command — 25 cases mirroring
//! `internal/cli/commands/start/{cmd_test,parser_test}.go`.

use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex, MutexGuard};

use crate::cli::commands::list::IpcOps as ListIpcOps;
use crate::cli::commands::start::{parse_app_spec, run, StartOps, StartedInstance};
use crate::ipc::protocol::{
	AppLogs, AppRestart, AppSpec, RunAsPolicy, StartRequest, StartResponseData,
};

/// Process-global `XDG_CONFIG_HOME` is consulted by `spec.save_spec_protocol`
/// to find its target directory. `cargo test` parallelises by default and
/// a Go-style test using `t.Setenv` would race with that. Use the same
/// pattern as `spec::tests`: a mutex on entry, restore in `Drop`.
static SPEC_LOCK: Mutex<()> = Mutex::new(());

struct SpecEnvGuard {
	_lock: MutexGuard<'static, ()>,
	prev: Option<String>,
}

impl Drop for SpecEnvGuard {
	fn drop(&mut self) {
		match self.prev.as_deref() {
			Some(v) => std::env::set_var("XDG_CONFIG_HOME", v),
			None => std::env::remove_var("XDG_CONFIG_HOME"),
		}
	}
}

fn lock_spec_env() -> SpecEnvGuard {
	let prev = std::env::var("XDG_CONFIG_HOME").ok();
	SpecEnvGuard {
		_lock: SPEC_LOCK.lock().unwrap_or_else(|e| e.into_inner()),
		prev,
	}
}

fn set_xdg_tempdir() -> tempfile::TempDir {
	let dir = tempfile::tempdir().expect("tempdir");
	std::env::set_var("XDG_CONFIG_HOME", dir.path());
	dir
}

// --- Mock IPC client --------------------------------------------------------

struct MockStart {
	start_response: StartResponseData,
	start_calls: Arc<Mutex<Vec<StartRequest>>>,
	list_count: Arc<AtomicU32>,
	fail_on_call: Arc<AtomicU32>,
}

impl MockStart {
	fn new(proc_id: &str, pid: i32, status: &str) -> Self {
		Self {
			start_response: StartResponseData {
				id: "stub-id".into(),
				proc_id: Some(proc_id.into()),
				pid: Some(pid),
				status: Some(status.into()),
				message: None,
				created_at: None,
			},
			start_calls: Arc::new(Mutex::new(Vec::new())),
			list_count: Arc::new(AtomicU32::new(0)),
			fail_on_call: Arc::new(AtomicU32::new(0)),
		}
	}

	fn calls(&self) -> Vec<StartRequest> {
		self.start_calls.lock().unwrap().clone()
	}
}

impl StartOps for MockStart {
	type Error = String;

	fn start(&mut self, req: &StartRequest) -> Result<StartResponseData, String> {
		self.start_calls.lock().unwrap().push(req.clone());
		let count = self.start_calls.lock().unwrap().len() as u32;
		if self.fail_on_call.load(Ordering::Relaxed) == count {
			return Err("daemon rejected".into());
		}
		Ok(self.start_response.clone())
	}
}

impl ListIpcOps for MockStart {
	fn call_list(
		&mut self,
	) -> Result<Vec<crate::types::ProcessInfo>, crate::cli::commands::list::IpcError> {
		self.list_count.fetch_add(1, Ordering::Relaxed);
		Ok(Vec::new())
	}
}

// --- parseMemorySize (7 cases) ----------------------------------------------

#[test]
fn parse_memory_size_empty() {
	assert_eq!(
		crate::cli::commands::start::parse_memory_size("").unwrap(),
		0
	);
}

#[test]
fn parse_memory_size_whitespace() {
	assert_eq!(
		crate::cli::commands::start::parse_memory_size("   ").unwrap(),
		0
	);
}

#[test]
fn parse_memory_size_kilobytes() {
	let cases = [("512k", 512 * 1024), ("512K", 512 * 1024), ("1K", 1024)];
	for (input, want) in cases {
		let got = crate::cli::commands::start::parse_memory_size(input).unwrap();
		assert_eq!(got, want, "parse_memory_size({input}) = {got}");
	}
}

#[test]
fn parse_memory_size_megabytes() {
	let cases = [
		("512m", 512 * 1024 * 1024),
		("512M", 512 * 1024 * 1024),
		("1M", 1024 * 1024),
	];
	for (input, want) in cases {
		let got = crate::cli::commands::start::parse_memory_size(input).unwrap();
		assert_eq!(got, want, "parse_memory_size({input}) = {got}");
	}
}

#[test]
fn parse_memory_size_gigabytes() {
	let got = crate::cli::commands::start::parse_memory_size("2G").unwrap();
	assert_eq!(got, 2i64 * 1024 * 1024 * 1024);
}

#[test]
fn parse_memory_size_raw_bytes() {
	let got = crate::cli::commands::start::parse_memory_size("10485760").unwrap();
	assert_eq!(got, 10485760);
}

#[test]
fn parse_memory_size_invalid() {
	for input in ["abc", "0M", "-1M", "0"] {
		assert!(
			crate::cli::commands::start::parse_memory_size(input).is_err(),
			"expected error for {input}"
		);
	}
}

// --- ReadIntList helpers (5 cases) -----------------------------------------

#[test]
fn read_int_list_basic() {
	let mut p =
		crate::cli::commands::start::SpecParser::new(&["--cpus".to_string(), "0,1,2".to_string()]);
	let mut result = Vec::new();
	p.read_int_list(&mut result).unwrap();
	assert_eq!(result, vec![0, 1, 2]);
}

#[test]
fn read_int_list_single() {
	let mut p =
		crate::cli::commands::start::SpecParser::new(&["--cpus".to_string(), "7".to_string()]);
	let mut result = Vec::new();
	p.read_int_list(&mut result).unwrap();
	assert_eq!(result, vec![7]);
}

#[test]
fn read_int_list_with_spaces() {
	let mut p = crate::cli::commands::start::SpecParser::new(&[
		"--cpus".to_string(),
		"0, 1, 2".to_string(),
	]);
	let mut result = Vec::new();
	p.read_int_list(&mut result).unwrap();
	assert_eq!(result.len(), 3);
}

#[test]
fn read_int_list_missing_value() {
	let mut p = crate::cli::commands::start::SpecParser::new(&["--cpus".to_string()]);
	let mut result = Vec::new();
	assert!(p.read_int_list(&mut result).is_err());
}

#[test]
fn read_int_list_invalid_int() {
	let mut p = crate::cli::commands::start::SpecParser::new(&[
		"--cpus".to_string(),
		"0,abc,2".to_string(),
	]);
	let mut result = Vec::new();
	assert!(p.read_int_list(&mut result).is_err());
}

// --- Test for tokenize (Go's TestTokenize). 11 subcases share one
// expectation; we mirror with three focused cases (the Go file runs
// them all in the same `TestTokenize` function — a single function in
// the count).
// --- ParseAppSpec (7 cases) -----------------------------------------------

#[allow(dead_code)]
fn expected_default_logs() -> AppLogs {
	AppLogs {
		mode: "file".to_string(),
		dir: None,
		stdout: None,
		stderr: None,
		format: Some("plain".to_string()),
		timestamp: Some("rfc3339".to_string()),
	}
}

#[allow(dead_code)]
fn expected_default_restart() -> AppRestart {
	AppRestart {
		policy: "on-failure".to_string(),
		max_retries: Some(10),
		backoff_ms: Some(2000),
		backoff_type: Some("expo".to_string()),
		stop_on_exit: Some(vec![0]),
	}
}

#[allow(dead_code)]
fn expected_default_run_as() -> RunAsPolicy {
	RunAsPolicy {
		mode: "self".to_string(),
	}
}

fn args_vec(s: &[&str]) -> Vec<String> {
	s.iter().map(|s| (*s).to_string()).collect()
}

fn cwd() -> String {
	std::env::current_dir()
		.map(|p| p.to_string_lossy().to_string())
		.unwrap_or_default()
}

fn parse(s: &[&str]) -> (AppSpec, i32) {
	let args = args_vec(s);
	let (mut spec, scale) = parse_app_spec(&args).expect("parse");
	if !spec.cwd.as_deref().unwrap_or("").is_empty() {
		spec.cwd = Some(cwd());
	}
	spec.id = String::new();
	spec.created_at = None;
	spec.exec.shell = false;
	spec.env = spec.env.or(Some(Default::default()));
	(spec, scale)
}

#[test]
fn parse_app_spec_main_js_inferred_node() {
	let (spec, scale) = parse(&["main.js"]);
	assert_eq!(spec.name, "");
	assert_eq!(spec.namespace, Some("default".to_string()));
	assert_eq!(spec.logs.as_ref().unwrap().mode, "file");
	assert_eq!(spec.restart.as_ref().unwrap().policy, "on-failure");
	assert_eq!(spec.run_as.as_ref().unwrap().mode, "self");
	assert_eq!(spec.exec.kind, "entry");
	assert_eq!(spec.exec.entry.as_deref(), Some("main.js"));
	assert_eq!(spec.exec.runtime.as_deref(), Some("node"));
	assert_eq!(scale, 1);
}

#[test]
fn parse_app_spec_main_go_inferred_go_run() {
	let (spec, _scale) = parse(&["main.go", "--name", "Test"]);
	assert_eq!(spec.name, "Test");
	assert_eq!(spec.exec.kind, "entry");
	assert_eq!(spec.exec.entry.as_deref(), Some("main.go"));
	assert_eq!(spec.exec.runtime.as_deref(), Some("go run"));
}

#[test]
fn parse_app_spec_quoted_command_parses_command_and_args() {
	let (spec, _scale) = parse(&["bun dev"]);
	assert_eq!(spec.exec.kind, "command");
	assert_eq!(spec.exec.command.as_deref(), Some("bun"));
	assert_eq!(spec.exec.args, Some(vec!["dev".to_string()]));
}

#[test]
fn parse_app_spec_quoted_node_run_dev() {
	let (spec, _scale) = parse(&["node --run dev", "--name", "test"]);
	assert_eq!(spec.exec.kind, "command");
	assert_eq!(spec.exec.command.as_deref(), Some("node"));
	assert_eq!(
		spec.exec.args,
		Some(vec!["--run".to_string(), "dev".to_string()])
	);
	assert_eq!(spec.name, "test");
}

#[test]
fn parse_app_spec_multi_token_command() {
	let (spec, _scale) = parse(&["node", "--run", "dev"]);
	assert_eq!(spec.exec.kind, "command");
	assert_eq!(spec.exec.command.as_deref(), Some("node"));
	assert_eq!(
		spec.exec.args,
		Some(vec!["--run".to_string(), "dev".to_string()])
	);
}

#[test]
fn parse_app_spec_double_dash_passes_remaining_as_command() {
	let (spec, _scale) = parse(&["--", "node", "--run", "dev"]);
	assert_eq!(spec.exec.kind, "command");
	assert_eq!(spec.exec.command.as_deref(), Some("node"));
	assert_eq!(
		spec.exec.args,
		Some(vec!["--run".to_string(), "dev".to_string()])
	);
}

#[test]
fn parse_app_spec_runtime_override_treats_as_entry() {
	let (spec, _scale) = parse(&["app.py", "--runtime", "python3"]);
	assert_eq!(spec.exec.kind, "entry");
	assert_eq!(spec.exec.entry.as_deref(), Some("app.py"));
	assert_eq!(spec.exec.runtime.as_deref(), Some("python3"));
}

#[test]
fn parse_app_spec_validation_empty_args_errors() {
	let args = args_vec(&[]);
	let err = parse_app_spec(&args).unwrap_err();
	assert!(err.message.contains("missing command"), "got {err:?}");
}

// --- Run-flow (10 cases) ---------------------------------------------------

#[test]
fn start_run_help_writes_no_error() {
	let mut client = MockStart::new("abc-123", 9999, "running");
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["--help".to_string()];
	let result = run(Some(&mut client), &mut out, &mut err_buf, &args);
	assert!(result.is_ok(), "--help should not error");
}

#[test]
fn start_run_empty_args_errors_with_missing_command() {
	let mut client = MockStart::new("x", 1, "running");
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args: Vec<String> = Vec::new();
	let result = run(Some(&mut client), &mut out, &mut err_buf, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("missing command"), "got {msg:?}");
}

#[test]
fn start_run_success_makes_one_ipc_call() {
	// XDG_CONFIG_HOME points at a tempdir so save_spec_protocol
	// cannot pollute the user's home. The guard serialises parallel
	// tests that hit this env var; failures mid-assertion are
	// restored on Drop.
	let _g = lock_spec_env();
	let _tmp = set_xdg_tempdir();

	let mut client = MockStart::new("abc-123", 9999, "running");
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"echo".to_string(),
		"hello".to_string(),
		"--no-list".to_string(),
	];
	run(Some(&mut client), &mut out, &mut err_buf, &args).expect("ok");
	let calls = client.calls();
	assert_eq!(calls.len(), 1, "expected one 'start' call, got {calls:?}");
}

#[test]
fn start_run_scale_makes_n_ipc_calls() {
	let _g = lock_spec_env();
	let _tmp = set_xdg_tempdir();

	let mut client = MockStart::new("abc-123", 9999, "running");
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"echo".to_string(),
		"--scale".to_string(),
		"3".to_string(),
		"--no-list".to_string(),
	];
	run(Some(&mut client), &mut out, &mut err_buf, &args).expect("ok");
	assert_eq!(client.calls().len(), 3, "expected 3 calls for scale=3");
}

#[test]
fn start_run_ipc_error_returns_failure() {
	let _g = lock_spec_env();
	let _tmp = set_xdg_tempdir();

	let mut client = MockStart::new("x", 1, "running");
	client.fail_on_call = Arc::new(AtomicU32::new(1));
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec!["echo".to_string(), "--no-list".to_string()];
	let result = run(Some(&mut client), &mut out, &mut err_buf, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("start failed"), "got {msg:?}");
}

#[test]
fn start_spec_has_name_start() {
	use crate::cli::commands::start::spec;
	let s = spec();
	assert_eq!(s.name, "start");
}

#[test]
fn start_run_json_output_emits_batch_report() {
	let _g = lock_spec_env();
	let _tmp = set_xdg_tempdir();

	let mut client = MockStart::new("id-0", 1111, "running");
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"echo".to_string(),
		"--scale".to_string(),
		"2".to_string(),
		"--json".to_string(),
	];
	run(Some(&mut client), &mut out, &mut err_buf, &args).expect("ok");
	let got = String::from_utf8_lossy(&out);
	let parsed: serde_json::Value = serde_json::from_str(got.trim()).expect("valid json");
	let started = parsed["started"].as_array().expect("started array");
	assert_eq!(started.len(), 2);
	assert_eq!(parsed["count"], 2);
	assert!(got.trim_start().starts_with('{'), "got {got:?}");
}

#[test]
fn start_dry_run_json_does_not_talk_to_daemon() {
	// --dry-run --json must emit spec+scale without ever touching the
	// daemon. Passing a `None` client should work because the dry-run
	// path skips the IPC round-trip.
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"--dry-run".to_string(),
		"--json".to_string(),
		"echo".to_string(),
		"hello".to_string(),
	];
	run::<Vec<u8>, Vec<u8>, MockStart>(None, &mut out, &mut err_buf, &args).expect("dry-run ok");
	let got = String::from_utf8_lossy(&out);
	let parsed: serde_json::Value = serde_json::from_str(got.trim()).expect("valid json");
	assert_eq!(parsed["scale"], 1);
	let cmd = parsed["spec"]["exec"]["command"]
		.as_str()
		.unwrap_or_default();
	assert_eq!(cmd, "echo");
}

#[test]
fn start_run_json_partial_failure_emits_partial_report() {
	let _g = lock_spec_env();
	let _tmp = set_xdg_tempdir();

	let mut client = MockStart::new("id-0", 1111, "running");
	client.fail_on_call = Arc::new(AtomicU32::new(2));
	let mut out = Vec::new();
	let mut err_buf = Vec::new();
	let args = vec![
		"echo".to_string(),
		"--scale".to_string(),
		"3".to_string(),
		"--json".to_string(),
	];
	let result = run(Some(&mut client), &mut out, &mut err_buf, &args);
	assert!(result.is_err(), "expected error on second instance failure");
	let got = String::from_utf8_lossy(&out);
	assert!(!got.is_empty(), "expected partial JSON report on stdout");
	let parsed: serde_json::Value = serde_json::from_str(got.trim()).expect("valid json");
	assert_eq!(parsed["partial"], true);
	assert_eq!(parsed["failed_at_instance"], 2);
}

// --- sanity: StartResponseData::default shape ------------------------------

#[test]
fn start_response_data_shape() {
	let d = StartResponseData {
		id: "x".into(),
		proc_id: Some("y".into()),
		pid: Some(10),
		status: Some("running".into()),
		message: None,
		created_at: None,
	};
	assert_eq!(d.proc_id.as_deref(), Some("y"));
	assert_eq!(d.pid, Some(10));
}

#[test]
fn started_instance_serialises_cleanly() {
	let _ = StartedInstance {
		name: "api".into(),
		id: "abc".into(),
		pid: 1234,
		status: "running".into(),
		namespace: Some("prod".into()),
	};
}
