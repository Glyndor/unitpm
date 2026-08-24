//! One lock for the process-wide environment the test suite mutates.
//!
//! `UNITPM_SOCKET` is a process-global, and four test modules write it:
//! `ipc::transport::server_tests`, `ipc::transport::socket_unix::tests`,
//! `daemon::handlers::tests::stack` and `daemon::handlers::tests::register_tests`.
//! Two of them held a `static ENV_LOCK` of their own and two held nothing, so
//! nothing serialised a module against any other module. A test could bind its
//! socket, have a test on another thread overwrite the variable, and then watch
//! for a path nobody was going to create.
//!
//! A per-module lock cannot fix this: exclusion only means something when every
//! writer takes the *same* lock. This is that lock.

use std::sync::{Mutex, MutexGuard};

static ENV_LOCK: Mutex<()> = Mutex::new(());

/// Blocks until this thread owns the shared environment.
///
/// Recovers from poisoning on purpose: a panicking test leaves the environment
/// dirty, and every caller restores what it touched on the way out, so turning
/// one failure into a cascade of poisoned-lock failures hides the original.
pub(crate) fn lock() -> MutexGuard<'static, ()> {
	ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner())
}
