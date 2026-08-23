//! End-to-end IPC stack fixture used by every test in this directory.
//!
//! The fixture is a `unitpmd`-style stack — manager, IPC server with
//! all handlers registered, and a connected client — bound to a temp
//! socket. The accompanying `EnvGuard` keeps the process-wide
//! `XDG_*` / `UNITPM_SOCKET` variables from leaking between parallel
//! tests, so two tests can each point at their own temp dir without
//! one re-pointing the env while the other is mid-write.

#![cfg(target_os = "linux")]

use std::sync::Arc;
use std::time::Duration;

use crate::daemon::audit::Logger;
use crate::daemon::handlers::register_handlers;
use crate::ipc::transport::{Client, Server};

use super::EnvGuard;

/// The bundled server + client + manager that a test gets from
/// [`setup`]. The `Server` lives until [`drop_stack`] is called or
/// the value drops, whichever comes first.
pub(crate) struct Stack {
	pub(crate) server: Server,
	pub(crate) client: Client,
	pub(crate) mgr: std::sync::Arc<std::sync::Mutex<crate::daemon::manager::Manager>>,
	pub(crate) _temp: tempfile::TempDir,
}

/// Wire a real `unitpmd`-style stack — manager, IPC server, registered
/// handlers, and a connected client — against a temp dir and a temp
/// socket. The [`EnvGuard`] keeps `XDG_CONFIG_HOME`, `XDG_STATE_HOME`,
/// `HOME`, and `UNITPM_SOCKET` from leaking between parallel tests.
pub(crate) fn setup() -> Stack {
	let _env = EnvGuard::new();
	let temp = tempfile::tempdir().expect("tempdir");
	std::env::set_var("XDG_CONFIG_HOME", temp.path());
	std::env::set_var("XDG_STATE_HOME", temp.path());
	std::env::set_var("HOME", temp.path());
	let socket = temp.path().join("unitpm.sock");
	std::env::set_var("UNITPM_SOCKET", &socket);

	let mgr = super::new_manager();
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

/// Drop a [`Stack`] in the right order: close the server first so the
/// accept loop ends, then the client, then drop the manager.
pub(crate) fn drop_stack(stack: Stack) {
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
