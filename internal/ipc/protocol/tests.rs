//! Tests for the protocol package. Mirrored from `protocol_test.go`.
//!
//! All twelve cases are gated to Linux because the Go test file carries the
//! `//go:build linux` constraint, even though the wire format itself is
//! platform-neutral — the daemon only runs on Linux in production, and tests
//! that no real call site exercises on other platforms were never going to be
//! meaningful there.

use serde_json::json;

use super::*;

#[test]
fn version_constant() {
	assert_eq!(VERSION, 1);
}

#[test]
fn status_constants() {
	assert_eq!(STATUS_ERROR, "error");
	assert_eq!(STATUS_SUCCESS, "success");
}

#[test]
fn request_json_round_trip() {
	let params = jsonx::marshal(&json!({ "id": "abc" })).expect("marshal params");
	let req = Request {
		version: 1,
		id: "req-123".into(),
		command: "stop".into(),
		params: Some(RawMessage::from_bytes(&params)),
	};
	let data = jsonx::marshal(&req).expect("marshal request");
	let got: Request = jsonx::unmarshal(&data).expect("unmarshal request");
	assert_eq!(got.version, 1);
	assert_eq!(got.id, "req-123");
	assert_eq!(got.command, "stop");
}

#[test]
fn response_error_case() {
	let resp = Response {
		version: 1,
		id: "req-1".into(),
		status: STATUS_ERROR.into(),
		result: None,
		error: Some(Box::new(Error {
			code: "ERR_NOT_FOUND".into(),
			message: "process not found".into(),
			data: None,
		})),
	};
	let data = jsonx::marshal(&resp).expect("marshal response");
	let got: Response = jsonx::unmarshal(&data).expect("unmarshal response");
	assert_eq!(got.status, STATUS_ERROR);
	let err = got.error.expect("error field should not be nil");
	assert_eq!(err.code, "ERR_NOT_FOUND");
}

#[test]
fn response_success_case() {
	let result = jsonx::marshal(&json!({ "id": "abc123" })).expect("marshal result");
	let resp = Response {
		version: 1,
		id: "req-2".into(),
		status: STATUS_SUCCESS.into(),
		result: Some(RawMessage::from_bytes(&result)),
		error: None,
	};
	let data = jsonx::marshal(&resp).expect("marshal response");
	let got: Response = jsonx::unmarshal(&data).expect("unmarshal response");
	assert_eq!(got.status, STATUS_SUCCESS);
	assert!(got.error.is_none(), "error field should be None on success");
}

#[test]
fn app_spec_json_round_trip() {
	let spec = AppSpec {
		version: 1,
		id: "uuid-001".into(),
		name: "myapp".into(),
		namespace: Some("production".into()),
		exec: AppExec {
			kind: "command".into(),
			command: Some("node".into()),
			args: Some(vec!["server.js".into(), "--port".into(), "8080".into()]),
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: Some("/app".into()),
		env: Some({
			let mut m = std::collections::BTreeMap::new();
			m.insert("PORT".into(), "8080".into());
			m
		}),
		env_file: Some(".env".into()),
		logs: Some(Box::new(AppLogs {
			mode: "file".into(),
			dir: Some("/var/log/glyndor/myapp".into()),
			stdout: Some("out.log".into()),
			stderr: Some("err.log".into()),
			format: Some("json".into()),
			timestamp: Some("rfc3339".into()),
		})),
		restart: Some(Box::new(AppRestart {
			policy: "on-failure".into(),
			max_retries: Some(3),
			backoff_ms: Some(500),
			backoff_type: Some("expo".into()),
			stop_on_exit: Some(vec![0, 2]),
		})),
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

	let data = jsonx::marshal(&spec).expect("marshal");
	let got: AppSpec = jsonx::unmarshal(&data).expect("unmarshal");

	assert_eq!(got.name, spec.name);
	assert_eq!(got.exec.command.as_deref(), Some("node"));
	let args = got.exec.args.as_ref().expect("args");
	assert_eq!(args.len(), 3);
	let logs = got.logs.as_ref().expect("logs");
	assert_eq!(logs.mode, "file");
	let restart = got.restart.as_ref().expect("restart");
	assert_eq!(restart.policy, "on-failure");
	let stop_on_exit = restart.stop_on_exit.as_ref().expect("stop_on_exit");
	assert_eq!(stop_on_exit.len(), 2);
	let run_as = got.run_as.as_ref().expect("runAs");
	assert_eq!(run_as.mode, "self");
}

#[test]
fn app_spec_omit_empty_fields() {
	let spec = AppSpec {
		version: 1,
		id: String::new(),
		name: "minimal".into(),
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
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	};
	let data = jsonx::marshal(&spec).expect("marshal");
	let v: serde_json::Value = serde_json::from_slice(&data).expect("parse");
	let obj = v.as_object().expect("object");
	for key in ["logs", "restart", "runAs", "namespace"] {
		assert!(
			!obj.contains_key(key),
			"field {key} should be omitted, got {data:?}",
		);
	}
}

#[test]
fn start_request_json_round_trip() {
	let req = StartRequest {
		protocol_version: 1,
		request_id: "start-001".into(),
		kind: "start".into(),
		spec: AppSpec {
			version: 1,
			id: String::new(),
			name: "api".into(),
			namespace: None,
			exec: AppExec {
				kind: "command".into(),
				command: Some("go".into()),
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
		},
	};
	let data = jsonx::marshal(&req).expect("marshal");
	let got: StartRequest = jsonx::unmarshal(&data).expect("unmarshal");
	assert_eq!(got.kind, "start");
	assert_eq!(got.spec.name, "api");
}

#[test]
fn start_response_ok_case() {
	let resp = StartResponse {
		protocol_version: 1,
		kind: "start".into(),
		request_id: "start-001".into(),
		ok: true,
		data: Some(Box::new(StartResponseData {
			id: "proc-123".into(),
			proc_id: None,
			pid: Some(42),
			status: Some("running".into()),
			message: None,
			created_at: None,
		})),
		error: None,
	};
	let data = jsonx::marshal(&resp).expect("marshal");
	let got: StartResponse = jsonx::unmarshal(&data).expect("unmarshal");
	assert!(got.ok, "expected ok=true");
	let data = got.data.expect("data should not be None");
	assert_eq!(data.pid, Some(42));
}

#[test]
fn remote_error_error() {
	let re = RemoteError {
		code: "ERR_TEST".into(),
		message: "something went wrong".into(),
		data: None,
	};
	let got = re.to_string();
	let want = "ipc error: [ERR_TEST] something went wrong";
	assert_eq!(got, want);
}

#[test]
fn remote_error_implements_error() {
	// Compile-time check: a `Box<dyn Error>` accepts `RemoteError`.
	fn _assert_error<E: std::error::Error>() {}
	_assert_error::<RemoteError>();
}

#[test]
fn start_response_error_case() {
	let resp = StartResponse {
		protocol_version: 1,
		kind: "start".into(),
		request_id: "start-002".into(),
		ok: false,
		data: None,
		error: Some(Box::new(StartError {
			code: "ERR_LIMIT".into(),
			message: "too many processes".into(),
		})),
	};
	let data = jsonx::marshal(&resp).expect("marshal");
	let got: StartResponse = jsonx::unmarshal(&data).expect("unmarshal");
	assert!(!got.ok, "expected ok=false");
	let err = got.error.expect("error should not be None");
	assert_eq!(err.code, "ERR_LIMIT");
}
