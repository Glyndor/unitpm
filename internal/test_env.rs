//! One lock for the process-wide state the test suite mutates.
//!
//! Environment variables and the euid override are process-global, and the
//! suite had six locks over them: two module-private `ENV_LOCK`s, a
//! `SPEC_LOCK`, a `TERM_LOCK`, and two modules with nothing at all.
//! `XDG_CONFIG_HOME` alone was written from five places under four different
//! locks. Exclusion only means something when every writer takes the *same*
//! lock, so none of it excluded anything: tests failed by whichever order the
//! runner happened to pick, which is what issue #62 was.
//!
//! It is reentrant on purpose. Merging the locks means a test can now reach a
//! second holder through a helper -- a terminal test that builds a handler
//! stack, say -- and a plain `Mutex` would deadlock the whole run rather than
//! fail one test. Reentrancy is per thread and the runner gives each test its
//! own, so this still serialises tests against each other.
//!
//! Not covered: `UNITPM_CACHE_PATH`, `UNITPM_TEST_INT_*` and
//! `UNITPM_IPC_RATE_*`. Each is written from exactly one module, so there is
//! no second writer to be excluded from.

use std::cell::{Cell, RefCell};
use std::sync::{Mutex, MutexGuard};

static STATE_LOCK: Mutex<()> = Mutex::new(());

thread_local! {
	/// The real guard, parked here while this thread owns the lock. A
	/// `MutexGuard` is not `Send`, which is exactly right: it never leaves the
	/// thread that took it.
	static HELD: RefCell<Option<MutexGuard<'static, ()>>> = const { RefCell::new(None) };
	static DEPTH: Cell<u32> = const { Cell::new(0) };
}

/// Held for as long as this thread needs the shared state. Reentrant: taking it
/// again on a thread that already holds it bumps a count instead of blocking,
/// and only the outermost one releases.
pub(crate) struct Guard(());

/// Blocks until this thread owns the shared process state.
///
/// Recovers from poisoning on purpose: a panicking test leaves the state dirty,
/// and every caller restores what it touched on the way out, so turning one
/// failure into a cascade of poisoned-lock failures would hide the original.
pub(crate) fn lock() -> Guard {
	let first = DEPTH.with(|d| {
		let n = d.get();
		d.set(n + 1);
		n == 0
	});
	if first {
		let g = STATE_LOCK.lock().unwrap_or_else(|e| e.into_inner());
		HELD.with(|h| *h.borrow_mut() = Some(g));
	}
	Guard(())
}

impl Drop for Guard {
	fn drop(&mut self) {
		let last = DEPTH.with(|d| {
			let n = d.get() - 1;
			d.set(n);
			n == 0
		});
		if last {
			HELD.with(|h| {
				h.borrow_mut().take();
			});
		}
	}
}
