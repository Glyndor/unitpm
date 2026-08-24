//! Tests for [`crate::daemon::handlers`]. Mirrors `service_test.go`,
//! `start_test.go`, `handlers_test.go`, and `handlers_integration_test.go`.
//!
//! Test files (in this directory):
//!
//! - [`service_tests`] — validation, env_file, cwd (7 cases).
//! - [`start_tests`] — `StartHandler` validation and execution (3 cases).
//! - [`register_tests`] — `register_handlers` wires every verb; the
//!   destructive-handler audit assertion (2 cases).
//! - [`integration_tests`] — end-to-end through a real Unix socket.
//!   These set `UNITPM_SOCKET` to a temp path and run a client against a
//!   started server (16 cases).
//!
//! Process-global environment variables are read by every test that
//! reaches into [`crate::spec`] or [`crate::ipc::transport::socket_unix`]:
//! `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `HOME`, and `UNITPM_SOCKET`.
//! Without serialisation, two tests can race to point the same variable at
//! their own temp directories and the loser finds an empty state. A
//! mutex + a `Drop`-restoring guard fix both halves — `cargo test`
//! parallelises by default while Go only does so when a test asks with
//! `t.Parallel()`, and a failing assertion unwinds the stack, so the
//! restore has to live in `Drop`, not at the end of the function.

#![cfg(target_os = "linux")]

use std::sync::MutexGuard;

/// Guards every environment variable the tests reach into. Acquired on
/// construction; restored on `Drop`. The restoration step is what makes
/// this survive a panicking test — without it, the next test would find
/// the variable pointing at a deleted temp directory.
pub(crate) struct EnvGuard {
	_held: MutexGuard<'static, ()>,
	prev: Vec<(&'static str, Option<String>)>,
}

impl EnvGuard {
	pub(crate) fn new() -> Self {
		// The shared lock, not one of this module's own: these tests write
		// UNITPM_SOCKET, and so do the transport tests. Two locks would have
		// excluded neither from the other.
		let held = crate::test_env::lock();
		let vars = ["XDG_CONFIG_HOME", "XDG_STATE_HOME", "HOME", "UNITPM_SOCKET"];
		let prev: Vec<(&'static str, Option<String>)> =
			vars.iter().map(|k| (*k, std::env::var(k).ok())).collect();
		Self { _held: held, prev }
	}
}

impl Drop for EnvGuard {
	fn drop(&mut self) {
		for (k, prev) in self.prev.iter().rev() {
			match prev {
				Some(v) => std::env::set_var(k, v),
				None => std::env::remove_var(k),
			}
		}
	}
}

pub(crate) fn self_identity() -> crate::ipc::transport::Identity {
	use crate::ipc::transport::Identity;
	let uid = std::env::var("UID")
		.or_else(|_| std::env::var("EUID"))
		.unwrap_or_else(|_| "1000".into());
	let gid = std::env::var("GID").unwrap_or_else(|_| "1000".into());
	Identity {
		uid: if uid.is_empty() { "1000".into() } else { uid },
		gid: if gid.is_empty() { "1000".into() } else { gid },
		pid: std::process::id() as i32,
	}
}

pub(crate) fn new_manager() -> crate::daemon::handlers::SharedManager {
	std::sync::Arc::new(std::sync::Mutex::new(crate::daemon::manager::Manager::new()))
}

/// Shared integration test scaffolding. Lives here (rather than inside
/// `integration_tests`) so the per-topic test files (`flush_tests`,
/// `proctree_tests`) can call `setup()` without crossing module
/// boundaries — `super::super::integration_tests::setup` would chain
/// through private modules.
pub(crate) mod stack;

mod flush_tests;
mod integration_tests;
mod proctree_tests;
mod register_tests;
mod service_tests;
mod start_tests;
