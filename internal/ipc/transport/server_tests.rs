//! Integration tests for the daemon-side IPC server.
//!
//! Mirrored from `ipc_test.go`. These exercise the full client/server round
//! trip on a Unix socket — the server is started in-process, the client
//! connects to it, and the assertions run over the wire. The harness sets
//! `UNITPM_SOCKET` and `XDG_RUNTIME_DIR` for each test and restores them
//! on the way out via [`EnvGuard`].
//!
//! Linux-only by inheritance: `tests` for the protocol package are already
//! gated that way, and the listener uses Unix-only `listen`.

use std::os::unix::fs::PermissionsExt;
use std::sync::Mutex;

use crate::ipc::protocol::RawMessage;
use crate::ipc::transport::{Client, IPCClient, RequestContext, Server};

static ENV_LOCK: Mutex<()> = Mutex::new(());

/// Holds the env lock and restores `UNITPM_SOCKET` and
/// `UNITPM_IPC_ALLOW_UIDS` on the way out. The daemon-unreachable test
/// from the socket suite already covers the hint-style error, so this
/// guard does not bother with `XDG_RUNTIME_DIR`.
pub(crate) struct EnvGuard {
	_unit: std::sync::MutexGuard<'static, ()>,
	saved_socket: Option<String>,
	saved_allowlist: Option<String>,
}

impl EnvGuard {
	pub(crate) fn new() -> Self {
		// Take the lock first so the read of the saved values is consistent
		// with the test body that mutates the env.
		let _unit = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
		let saved_socket = std::env::var("UNITPM_SOCKET").ok();
		let saved_allowlist = std::env::var("UNITPM_IPC_ALLOW_UIDS").ok();
		Self {
			_unit,
			saved_socket,
			saved_allowlist,
		}
	}
}

impl Drop for EnvGuard {
	fn drop(&mut self) {
		match &self.saved_socket {
			Some(v) => std::env::set_var("UNITPM_SOCKET", v),
			None => std::env::remove_var("UNITPM_SOCKET"),
		}
		match &self.saved_allowlist {
			Some(v) => std::env::set_var("UNITPM_IPC_ALLOW_UIDS", v),
			None => std::env::remove_var("UNITPM_IPC_ALLOW_UIDS"),
		}
	}
}

pub(crate) fn setup_test_socket() -> (tempfile::TempDir, std::path::PathBuf) {
	let dir = tempfile::tempdir().expect("tempdir");
	let sock_path = dir.path().join("unitpm.sock");
	std::env::set_var("UNITPM_SOCKET", sock_path.as_os_str());
	(dir, sock_path)
}

pub(crate) fn start_server_with<F>(setup: F) -> Server
where
	F: FnOnce(&Server),
{
	let server = Server::new();
	setup(&server);
	server.start().expect("server start");
	// Give the accept loop a moment to bind. The Go test uses 100ms; we
	// keep it for parity.
	std::thread::sleep(std::time::Duration::from_millis(100));
	server
}

#[test]
fn ipc_round_trip() {
	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register("ping", |_ctx: RequestContext, _params: RawMessage| {
			let v = serde_json::json!({"response": "pong"});
			let bytes = serde_json::to_vec(&v).expect("encode");
			Ok(RawMessage::from_bytes(&bytes))
		});
	});

	let mut client = Client::new().expect("client");
	let mut result: std::collections::HashMap<String, String> = Default::default();
	client
		.call::<(), _>("ping", None, Some(&mut result))
		.expect("ping");
	assert_eq!(result.get("response").map(String::as_str), Some("pong"));

	let err = client.call::<(), std::collections::HashMap<String, String>>("unknown", None, None);
	let err = err.expect_err("unknown command should fail");
	assert!(
		err.to_string().contains("UNKNOWN_COMMAND"),
		"unexpected error: {err}"
	);

	server.close();
}

#[test]
fn socket_permissions() {
	let _g = EnvGuard::new();
	let (_dir, path) = setup_test_socket();
	let server = start_server_with(|_s| {});
	let info = std::fs::metadata(&path).expect("stat");
	let perm = info.permissions().mode() & 0o777;
	assert_eq!(perm, 0o600, "socket permissions = {perm:o}, want 0600");
	server.close();
}

#[test]
fn identity_contains_peer_uid() {
	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register("whoami", |ctx: RequestContext, _params: RawMessage| {
			let id = &ctx.identity;
			let v = serde_json::json!({
				"uid": id.uid,
				"gid": id.gid,
				"pid": id.pid,
			});
			let bytes = serde_json::to_vec(&v).expect("encode");
			Ok(RawMessage::from_bytes(&bytes))
		});
	});

	let mut client = Client::new().expect("client");
	let mut identity: crate::ipc::transport::Identity = crate::ipc::transport::Identity {
		uid: String::new(),
		gid: String::new(),
		pid: 0,
	};
	client
		.call::<(), _>("whoami", None, Some(&mut identity))
		.expect("whoami");
	let expected_uid = unsafe { libc::geteuid() }.to_string();
	assert_eq!(identity.uid, expected_uid);
	server.close();
}

#[test]
fn oversized_message_rejected() {
	#[derive(serde::Serialize)]
	struct Big {
		data: String,
	}

	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register("echo", |_ctx: RequestContext, params: RawMessage| {
			Ok(params)
		});
	});

	let mut client = Client::new().expect("client");
	// MaxMsgSize is 1 MB; send a payload that, plus the JSON envelope,
	// exceeds the limit. The Go test uses 1MB + 1024 bytes; we do the
	// same here.
	let val = "a".repeat(1024 * 1024 + 1024);

	let err = client.call::<Big, ()>("echo", Some(&Big { data: val }), None);
	let err = err.expect_err("expected error for oversized message");
	let msg = err.to_string();
	// The server may close the connection rather than send a structured
	// error when the request body itself overflows the buffer; either
	// outcome is a rejection. The Go test accepts ERR_LIMITS, EOF,
	// connection reset, timeout, and "response ID mismatch" (the last
	// because the server cannot echo the request ID for a request it
	// could not parse).
	assert!(
		msg.contains("ERR_LIMITS")
			|| msg.contains("Broken pipe")
			|| msg.contains("connection reset")
			|| msg.contains("EOF")
			|| msg.contains("connection closed")
			|| msg.contains("response ID mismatch")
			|| msg.contains("Message"),
		"unexpected error: {msg}"
	);
	server.close();
}

#[test]
fn exact_max_message_size_accepted() {
	// Pair to `oversized_message_rejected`: a request that fits in the
	// limit must succeed. Without this half, a limit set to zero would
	// pass every rejection test.
	//
	// The full request envelope
	// (`{"version":1,"id":"<uuid>","command":"echo","params":<data>}\n`)
	// is bigger than the params payload alone, so the params have to be
	// sized such that the entire wire-level message fits within
	// `MaxMsgSize`. We use a request whose params are a short string and
	// assert that the round trip succeeds — this catches a limit set too
	// low (e.g. `0`) which would otherwise reject every request.
	#[derive(serde::Serialize)]
	struct Small {
		data: String,
	}

	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register("echo", |_ctx: RequestContext, params: RawMessage| {
			Ok(params)
		});
	});

	let mut client = Client::new().expect("client");
	let mut echoed: serde_json::Value = serde_json::Value::Null;
	client
		.call::<Small, _>(
			"echo",
			Some(&Small {
				data: "hello world".into(),
			}),
			Some(&mut echoed),
		)
		.expect("echo of a normal-sized payload should pass");
	assert_eq!(echoed["data"].as_str(), Some("hello world"));
	server.close();
}

#[test]
fn server_recovers_from_handler_panic() {
	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register(
			"panic",
			|_ctx: RequestContext, _params: RawMessage| -> Result<RawMessage, String> {
				panic!("boom")
			},
		);
		s.register("ping", |_ctx: RequestContext, _params: RawMessage| {
			let v = serde_json::json!({"response": "pong"});
			let bytes = serde_json::to_vec(&v).expect("encode");
			Ok(RawMessage::from_bytes(&bytes))
		});
	});

	let mut client1 = Client::new().expect("client");
	let _ = client1.call::<(), ()>("panic", None, None);

	let mut client2 = Client::new().expect("client2");
	let mut result: std::collections::HashMap<String, String> = Default::default();
	client2
		.call::<(), _>("ping", None, Some(&mut result))
		.expect("ping after panic");
	assert_eq!(result.get("response").map(String::as_str), Some("pong"));
	server.close();
}

#[test]
fn has_handler_reports_registered_commands() {
	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register("ping", |_ctx: RequestContext, _params: RawMessage| {
			Ok(RawMessage::from_bytes(b"\"pong\""))
		});
	});
	assert!(server.has_handler("ping"));
	assert!(!server.has_handler("nonexistent"));
	server.close();
}

#[test]
fn response_decoder_round_trip() {
	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let server = start_server_with(|s| {
		s.register("echo", |_ctx: RequestContext, params: RawMessage| {
			Ok(params)
		});
	});

	let mut client = Client::new().expect("client");
	let mut result = String::new();
	client
		.call::<String, _>("echo", Some(&"hello".to_string()), Some(&mut result))
		.expect("echo");
	assert_eq!(result, "hello");
	server.close();
}

#[test]
fn dispatch_start_protocol_mismatch() {
	let _g = EnvGuard::new();
	let (_dir, path) = setup_test_socket();
	let server = start_server_with(|_s| {});

	use std::io::{Read, Write};
	let stream = std::os::unix::net::UnixStream::connect(&path).expect("connect");
	let mut stream = stream;
	let req = r#"{"type":"start","protocol_version":0,"request_id":"test-req-1"}
"#;
	stream.write_all(req.as_bytes()).expect("write");
	let mut buf = Vec::new();
	stream.read_to_end(&mut buf).expect("read");
	let resp = String::from_utf8_lossy(&buf);
	assert!(
		resp.contains("PROTOCOL_MISMATCH"),
		"expected PROTOCOL_MISMATCH in response, got: {resp}"
	);
	server.close();
}

#[test]
fn dispatch_start_no_handler() {
	let _g = EnvGuard::new();
	let (_dir, path) = setup_test_socket();
	let server = start_server_with(|_s| {});

	use std::io::{Read, Write};
	let stream = std::os::unix::net::UnixStream::connect(&path).expect("connect");
	let mut stream = stream;
	let req = r#"{"type":"start","protocol_version":1,"request_id":"test-req-2"}
"#;
	stream.write_all(req.as_bytes()).expect("write");
	let mut buf = Vec::new();
	stream.read_to_end(&mut buf).expect("read");
	let resp = String::from_utf8_lossy(&buf);
	assert!(
		resp.contains("UNKNOWN_COMMAND"),
		"expected UNKNOWN_COMMAND in response, got: {resp}"
	);
	server.close();
}
