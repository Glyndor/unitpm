//! HTTP test server for the updater tests.
//!
//! A tiny single-threaded accept loop bound to `127.0.0.1:0` that hands out
//! canned responses in FIFO order. The harness is paranoid about exit
//! conditions: every read has a deadline, every connect has one, and the
//! accept loop checks a shutdown flag on every iteration. A request that
//! arrives after the queue is exhausted gets a definite 500 response —
//! never a hang. Drop sets the shutdown flag and joins the accept thread,
//! so a test cannot leak the port to the next one.

use std::collections::VecDeque;
use std::io::{ErrorKind, Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

/// Lightweight HTTP test server. Hands out canned responses in FIFO order;
/// returns 500 once the queue is exhausted so callers never hang on a
/// missing reply.
pub(super) struct TestServer {
	addr: SocketAddr,
	shutdown: Arc<AtomicBool>,
	handle: Option<thread::JoinHandle<()>>,
}

const READ_TIMEOUT: Duration = Duration::from_secs(2);
const WRITE_TIMEOUT: Duration = Duration::from_secs(2);
const POLL_INTERVAL: Duration = Duration::from_millis(10);

impl TestServer {
	pub(super) fn new(responses: Vec<Vec<u8>>) -> Self {
		let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
		let addr = listener.local_addr().expect("addr");
		// Nonblocking on the listener so the accept loop can detect shutdown
		// without blocking. Accepted streams will be flipped back to blocking
		// so reads honour the read timeout.
		listener.set_nonblocking(true).expect("nonblocking");
		let queue = Arc::new(Mutex::new(VecDeque::from(responses)));
		let shutdown = Arc::new(AtomicBool::new(false));
		let shutdown_t = shutdown.clone();
		let handle = thread::Builder::new()
			.name("updater-test-server".into())
			.spawn(move || serve_loop(listener, queue, shutdown_t))
			.expect("spawn");
		Self {
			addr,
			shutdown,
			handle: Some(handle),
		}
	}

	pub(super) fn url(&self, path: &str) -> String {
		format!("http://{}/{}", self.addr, path.trim_start_matches('/'))
	}
}

impl Drop for TestServer {
	fn drop(&mut self) {
		// Flip the flag first so the accept loop exits at the top of its next
		// iteration rather than mid-read on a half-arrived connection.
		self.shutdown.store(true, Ordering::SeqCst);
		if let Some(h) = self.handle.take() {
			// A stuck inner read will time out at READ_TIMEOUT — bounded.
			let _ = h.join();
		}
	}
}

fn serve_loop(
	listener: TcpListener,
	queue: Arc<Mutex<VecDeque<Vec<u8>>>>,
	shutdown: Arc<AtomicBool>,
) {
	loop {
		if shutdown.load(Ordering::SeqCst) {
			return;
		}
		match listener.accept() {
			Ok((stream, _)) => {
				handle_connection(stream, &queue);
			}
			Err(ref e) if e.kind() == ErrorKind::WouldBlock => {
				thread::sleep(POLL_INTERVAL);
			}
			Err(_) => return,
		}
	}
}

fn handle_connection(mut stream: TcpStream, queue: &Arc<Mutex<VecDeque<Vec<u8>>>>) {
	// Accepted streams inherit the listener's nonblocking flag on Linux.
	// Flip back to blocking so the read timeout actually fires; on a
	// nonblocking stream, read would return WouldBlock immediately and
	// we'd lose the request.
	let _ = stream.set_nonblocking(false);
	stream.set_read_timeout(Some(READ_TIMEOUT)).ok();
	stream.set_write_timeout(Some(WRITE_TIMEOUT)).ok();

	let mut buf = Vec::with_capacity(512);
	let mut chunk = [0u8; 1024];
	loop {
		match stream.read(&mut chunk) {
			Ok(0) => break,
			Ok(n) => {
				buf.extend_from_slice(&chunk[..n]);
				if buf.windows(4).any(|w| w == b"\r\n\r\n") || buf.len() > 64 * 1024 {
					break;
				}
			}
			Err(ref e) if e.kind() == ErrorKind::WouldBlock || e.kind() == ErrorKind::TimedOut => {
				break;
			}
			Err(_) => break,
		}
	}

	// The path is mostly irrelevant — we set the URLs explicitly to the
	// server root in every test. Pop FIFO so multi-response tests get each
	// reply in the order they were queued. Each entry is the FULL response
	// (already wrapped via `build_response`), so tests that want non-200
	// statuses can use `json_response(404, …)` or `json_response(500, …)`
	// to ship a properly framed reply.
	let next = queue.lock().unwrap().pop_front();
	let response =
		next.unwrap_or_else(|| build_response(500, "text/plain", b"test server: queue exhausted"));

	let _ = stream.write_all(&response);
	let _ = stream.flush();
}

/// Build a complete HTTP/1.1 response. Always closes the connection so the
/// client opens a fresh one for the next request.
pub(super) fn build_response(status: u16, ct: &str, body: &[u8]) -> Vec<u8> {
	let reason = match status {
		200 => "OK",
		404 => "Not Found",
		500 => "Internal Server Error",
		_ => "Status",
	};
	let mut r = Vec::new();
	r.extend_from_slice(
		format!(
			"HTTP/1.1 {status} {reason}\r\nContent-Type: {ct}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
			body.len()
		)
		.as_bytes(),
	);
	r.extend_from_slice(body);
	r
}

/// Wrap a body as a JSON-typed response.
pub(super) fn json_response(status: u16, body: &[u8]) -> Vec<u8> {
	build_response(status, "application/json", body)
}

/// Serialise a JSON value into bytes for the response body.
pub(super) fn json_body<T: serde::Serialize>(v: &T) -> Vec<u8> {
	serde_json::to_vec(v).expect("json")
}
