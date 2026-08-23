//! Tests for [`crate::daemon::handlers::start`]. Mirrors `start_test.go`.
//!
//! Three top-level test functions:
//!
//! - [`start_handler_validation`] — drives `StartHandler` with a context
//!   that has peer identity attached, expects each rejection to carry the
//!   named wire code prefix.
//! - [`start_handler_execution`] — happy path: a real command (`go
//!   version`) runs through the manager and the response carries a
//!   non-zero PID.
//! - [`start_handler_shell_denied_in_privileged_mode`] — the security
//!   gate: `shell: true` against a privileged daemon must be refused with
//!   `ERR_UNSUPPORTED`.

#![cfg(target_os = "linux")]

use std::collections::BTreeMap;

use crate::daemon::handlers::start::start_handler;
use crate::ipc::protocol::{AppExec, AppSpec, AppStop, RunAsPolicy, StartRequest};
use crate::ipc::transport::{Identity, RequestContext};
use crate::jsonx;
use uuid::Uuid;

use super::{new_manager, EnvGuard};

fn ctx_with_identity() -> RequestContext {
	RequestContext {
		identity: Identity {
			uid: "1000".into(),
			gid: "1000".into(),
			pid: 1234,
		},
	}
}

fn base_app_spec(command: &str) -> AppSpec {
	AppSpec {
		version: 1,
		id: Uuid::now_v7().to_string(),
		name: "n".into(),
		namespace: None,
		exec: AppExec {
			kind: "command".into(),
			command: Some(command.into()),
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

fn marshal_request(spec: &AppSpec) -> jsonx::RawMessage {
	let req = StartRequest {
		protocol_version: 1,
		request_id: spec.id.clone(),
		kind: "start".into(),
		spec: spec.clone(),
	};
	let bytes = jsonx::marshal(&req).expect("marshal");
	jsonx::RawMessage::from_bytes(&bytes)
}

#[test]
fn start_handler_validation() {
	let _env = EnvGuard::new();
	let mgr = new_manager();
	let handler = start_handler(mgr, false);
	let ctx = ctx_with_identity();

	struct Case {
		name: &'static str,
		spec: AppSpec,
		want_err: bool,
		err_code: &'static str,
	}

	let cases = vec![
		Case {
			name: "valid self",
			spec: base_app_spec("echo"),
			want_err: false,
			err_code: "",
		},
		Case {
			name: "missing exec type",
			spec: AppSpec {
				exec: AppExec {
					kind: "".into(),
					command: None,
					args: None,
					entry: None,
					runtime: None,
					shell: false,
				},
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_BAD_REQUEST",
		},
		Case {
			name: "missing command",
			spec: AppSpec {
				exec: AppExec {
					kind: "command".into(),
					command: None,
					args: None,
					entry: None,
					runtime: None,
					shell: false,
				},
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_BAD_REQUEST",
		},
		Case {
			name: "too many args",
			spec: AppSpec {
				exec: AppExec {
					kind: "command".into(),
					command: Some("echo".into()),
					args: Some(vec!["a".to_string(); 300]),
					entry: None,
					runtime: None,
					shell: false,
				},
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_LIMITS",
		},
		Case {
			name: "cmd too long",
			spec: AppSpec {
				exec: AppExec {
					kind: "command".into(),
					command: Some("a".repeat(4097)),
					args: None,
					entry: None,
					runtime: None,
					shell: false,
				},
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_LIMITS",
		},
		Case {
			name: "env too many",
			spec: AppSpec {
				env: Some(BTreeMap::new()),
				..AppSpec {
					exec: AppExec {
						kind: "command".into(),
						command: Some("echo".into()),
						args: None,
						entry: None,
						runtime: None,
						shell: false,
					},
					..base_app_spec("echo")
				}
			},
			want_err: true,
			err_code: "ERR_LIMITS",
		},
		Case {
			name: "invalid name",
			spec: AppSpec {
				name: "Invalid;Name".into(),
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_BAD_REQUEST",
		},
		Case {
			name: "invalid cwd",
			spec: AppSpec {
				cwd: Some("/path/to/nonexistent/directory".into()),
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_BAD_REQUEST",
		},
		Case {
			name: "app_user unsupported",
			spec: AppSpec {
				run_as: Some(Box::new(RunAsPolicy {
					mode: "app_user".into(),
				})),
				..base_app_spec("echo")
			},
			want_err: true,
			err_code: "ERR_UNSUPPORTED",
		},
	];

	for c in cases {
		// env_too_many needs > 128 env entries; the BTreeMap above is empty
		// so the case will currently fail to match. Build a proper oversized
		// map inline:
		let spec = if c.name == "env too many" {
			let mut m = BTreeMap::new();
			for i in 0..129 {
				m.insert(format!("k{i}"), "v".into());
			}
			AppSpec {
				env: Some(m),
				..c.spec.clone()
			}
		} else {
			c.spec.clone()
		};
		let params = marshal_request(&spec);
		let result = handler(ctx.clone(), params);
		match (c.want_err, &result) {
			(true, Ok(_)) => panic!("{}: expected error, got ok", c.name),
			(false, Err(e)) => panic!("{}: unexpected error {e}", c.name),
			(true, Err(e)) => {
				if !c.err_code.is_empty() && !e.contains(c.err_code) {
					panic!("{}: err = {e:?}, want code {:?}", c.name, c.err_code);
				}
			}
			(false, Ok(_)) => {}
		}
	}
}

#[test]
fn start_handler_execution() {
	let _env = EnvGuard::new();
	let mgr = new_manager();
	let handler = start_handler(mgr, false);
	let ctx = ctx_with_identity();

	let spec = AppSpec {
		exec: AppExec {
			kind: "command".into(),
			command: Some("true".into()),
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
		..base_app_spec("true")
	};
	let params = marshal_request(&spec);
	let raw = handler(ctx, params).expect("start ok");
	let resp: crate::ipc::protocol::StartResponseData =
		jsonx::unmarshal(raw.as_bytes()).expect("decode");
	assert_eq!(resp.id, spec.id);
	let pid = resp.pid.expect("pid");
	assert!(pid > 0, "expected positive PID, got {pid}");
}

#[test]
fn start_handler_shell_denied_in_privileged_mode() {
	let _env = EnvGuard::new();
	let mgr = new_manager();
	let handler = start_handler(mgr, true);
	let ctx = ctx_with_identity();

	let mut spec = base_app_spec("echo");
	spec.exec.shell = true;
	spec.exec.args = Some(vec!["hello".into()]);

	let params = marshal_request(&spec);
	let err = handler(ctx, params).expect_err("shell should be refused in privileged mode");
	assert!(
		err.contains("ERR_UNSUPPORTED"),
		"expected ERR_UNSUPPORTED, got {err:?}"
	);
}

#[allow(dead_code)]
fn _stop_signal_field() -> Option<AppStop> {
	None
}
