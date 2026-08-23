//! Security-control integration tests for the IPC server.
//!
//! Each test exercises a control the daemon relies on for security
//! (`MaxConnections`, the per-UID rate limit, the `UNITPM_IPC_ALLOW_UIDS`
//! allowlist). Removing the control from the implementation should turn
//! the matching test red; that is how the tests are kept honest.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use crate::ipc::protocol::RawMessage;
use crate::ipc::transport::ratelimit::RateLimiter;
use crate::ipc::transport::{Client, IPCClient, RequestContext, Server};

use super::server_tests::{setup_test_socket, start_server_with, EnvGuard};

#[test]
fn ipc_allowlist_uids_allowed() {
	let _g = EnvGuard::new();
	let uid = unsafe { libc::geteuid() };
	std::env::set_var("UNITPM_IPC_ALLOW_UIDS", uid.to_string());
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
		.expect("ping with allowlist");
	assert_eq!(result.get("response").map(String::as_str), Some("pong"));
	std::env::remove_var("UNITPM_IPC_ALLOW_UIDS");
	server.close();
}

#[test]
fn ipc_allowlist_uids_denied() {
	let _g = EnvGuard::new();
	std::env::set_var("UNITPM_IPC_ALLOW_UIDS", "999999");
	let server = start_server_with(|s| {
		s.register("ping", |_ctx: RequestContext, _params: RawMessage| {
			let v = serde_json::json!({"response": "pong"});
			let bytes = serde_json::to_vec(&v).expect("encode");
			Ok(RawMessage::from_bytes(&bytes))
		});
	});

	let mut client = Client::new().expect("client");
	let err = client.call::<(), std::collections::HashMap<String, String>>("ping", None, None);
	let err = err.expect_err("unauthorized client should be denied");
	let msg = err.to_string();
	assert!(
		msg.contains("unauthorized")
			|| msg.contains("permission")
			|| msg.contains("denied")
			|| msg.contains("EOF")
			|| msg.contains("reset")
			|| msg.contains("closed"),
		"unexpected error for unauthorized client: {msg}"
	);
	std::env::remove_var("UNITPM_IPC_ALLOW_UIDS");
	server.close();
}

#[test]
fn max_connections_rejects_when_exceeded() {
	// Builds a server whose handler blocks until told to proceed. Open
	// `MaxConnections` connections, fire each handler, then open one more
	// and confirm the cap rejects it. Removing the `try_acquire` call
	// lets every concurrent connection through and turns this test red.
	let _g = EnvGuard::new();
	let (_dir, path) = setup_test_socket();
	let proceed = Arc::new(std::sync::Barrier::new(
		crate::ipc::transport::MaxConnections + 1,
	));
	let entered = Arc::new(AtomicUsize::new(0));

	let server = start_server_with(|s| {
		let proceed = proceed.clone();
		let entered = entered.clone();
		s.register("ping", move |_ctx: RequestContext, _params: RawMessage| {
			entered.fetch_add(1, Ordering::SeqCst);
			proceed.wait();
			let v = serde_json::json!({"ok": true});
			let bytes = serde_json::to_vec(&v).expect("encode");
			Ok(RawMessage::from_bytes(&bytes))
		});
	});

	let mut held: Vec<()> = Vec::new();
	for _ in 0..crate::ipc::transport::MaxConnections {
		let mut client =
			crate::ipc::transport::Client::connect_to(path.as_os_str()).expect("client");
		std::thread::spawn(move || {
			let mut result: serde_json::Value = serde_json::Value::Null;
			let _ = client.call::<(), _>("ping", None, Some(&mut result));
		});
		held.push(());
	}

	let deadline = std::time::Instant::now() + std::time::Duration::from_secs(2);
	while entered.load(Ordering::SeqCst) < crate::ipc::transport::MaxConnections
		&& std::time::Instant::now() < deadline
	{
		std::thread::sleep(std::time::Duration::from_millis(10));
	}
	assert_eq!(
		entered.load(Ordering::SeqCst),
		crate::ipc::transport::MaxConnections,
		"expected all MaxConnections handlers to be parked at the barrier, got {}",
		entered.load(Ordering::SeqCst)
	);

	let extra = std::os::unix::net::UnixStream::connect(path.as_os_str()).expect("connect extra");
	let mut extra = extra;
	use std::io::{Read, Write};
	extra.write_all(b"{}\n").expect("write extra");
	extra.flush().ok();
	extra
		.set_read_timeout(Some(std::time::Duration::from_millis(500)))
		.ok();
	let mut buf = Vec::new();
	let _ = extra.read_to_end(&mut buf);
	assert!(
		buf.is_empty(),
		"server should have closed extra connection without response, got {} bytes: {:?}",
		buf.len(),
		String::from_utf8_lossy(&buf)
	);

	proceed.wait();
	drop(held);
	server.close();
}

#[test]
fn rate_limit_rejects_when_burst_exhausted() {
	// Builds a server with a tiny rate limit (3 tokens, 0 refill) and
	// fires 5 calls in quick succession. The first 3 should succeed; the
	// rest must return ERR_RATE_LIMIT. Removing the rate-limit call in
	// the server's handle loop will let all 5 succeed and turn this test
	// red.
	let _g = EnvGuard::new();
	let (_dir, _path) = setup_test_socket();
	let rl = std::sync::Arc::new(RateLimiter::with_capacity_and_refill(3.0, 0.0));
	let server = Server::with_rate_limit(rl);
	server.register("ping", |_ctx: RequestContext, _params: RawMessage| {
		let v = serde_json::json!({"ok": true});
		let bytes = serde_json::to_vec(&v).expect("encode");
		Ok(RawMessage::from_bytes(&bytes))
	});
	server.start().expect("server start");
	std::thread::sleep(std::time::Duration::from_millis(100));

	let mut client = Client::new().expect("client");
	let mut rate_limited = 0;
	let mut succeeded = 0;
	for _ in 0..5 {
		let mut result: serde_json::Value = serde_json::Value::Null;
		match client.call::<(), _>("ping", None, Some(&mut result)) {
			Ok(_) => succeeded += 1,
			Err(e) if e.to_string().contains("ERR_RATE_LIMIT") => rate_limited += 1,
			Err(e) => panic!("unexpected error: {e}"),
		}
	}
	assert_eq!(succeeded, 3, "first 3 should pass, got {succeeded}");
	assert_eq!(
		rate_limited, 2,
		"last 2 should be rate-limited, got {rate_limited}"
	);
	server.close();
}
