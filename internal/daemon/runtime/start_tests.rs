//! Tests for `configure_process_isolation`. Mirrors `start_linux_test.go`.
//!
//! Three cases are preserved: `self` mode passes and sets `set_pgid`,
//! empty mode passes through the default branch (which the Go code calls
//! the `default` switch arm), and the two reserved modes (`app_user`,
//! `explicit_user`) are rejected with an `ERR_UNSUPPORTED`-prefixed
//! message.

use crate::daemon::runtime::start::{configure_process_isolation, IsolationError};
use crate::ipc::protocol::RunAsPolicy;

#[test]
fn self_mode_passes() {
	let attr = configure_process_isolation(&RunAsPolicy {
		mode: "self".into(),
	})
	.expect("self mode");
	assert!(attr.set_pgid, "Setpgid must be true even for self mode");
}

#[test]
fn empty_mode_passes() {
	let attr = configure_process_isolation(&RunAsPolicy {
		mode: String::new(),
	})
	.expect("empty mode");
	assert!(attr.set_pgid, "Setpgid must be true for the default branch");
}

#[test]
fn reserved_modes_rejected() {
	for mode in ["app_user", "explicit_user"] {
		let err = configure_process_isolation(&RunAsPolicy { mode: mode.into() })
			.expect_err(&format!("mode {mode:?} should be rejected"));
		assert_eq!(err, IsolationError { mode: mode.into() });
		assert!(
			err.to_string().contains("not implemented yet"),
			"mode {mode:?}: unexpected error {err}"
		);
	}
}
