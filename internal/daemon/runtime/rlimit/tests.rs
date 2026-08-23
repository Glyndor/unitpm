//! Tests for the `rlimit` wrapper. Mirrors `rlimit_linux_test.go`.
//!
//! Both Go cases are preserved:
//!
//! - `TestApply_Zero_NoChange`: all-zero limits must not touch any rlimit.
//! - `TestApply_MaxFiles_LowersSoft`: setting `MaxFiles` to half the current
//!   soft limit must lower the soft cap and match the hard cap.
//!
//! Both tests are gated to Linux via the module containing `mod.rs`'s
//! `#[cfg(target_os = "linux")]` (the parent module compiles the `tests`
//! submodule only on Linux, mirroring the Go `//go:build linux` constraint).

use super::{apply, Limits};

#[test]
fn apply_zero_limits_no_change() {
	let mut before = std::mem::MaybeUninit::<libc::rlimit>::uninit();
	let r = unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, before.as_mut_ptr()) };
	assert_eq!(
		r,
		0,
		"getrlimit failed: {}",
		std::io::Error::last_os_error()
	);
	let before = unsafe { before.assume_init() };

	apply(&Limits::default()).expect("zero limits apply");

	let mut after = std::mem::MaybeUninit::<libc::rlimit>::uninit();
	let r = unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, after.as_mut_ptr()) };
	assert_eq!(
		r,
		0,
		"getrlimit failed: {}",
		std::io::Error::last_os_error()
	);
	let after = unsafe { after.assume_init() };

	assert_eq!(
		before.rlim_cur, after.rlim_cur,
		"RLIMIT_NOFILE soft changed: {} -> {}",
		before.rlim_cur, after.rlim_cur
	);
	assert_eq!(
		before.rlim_max, after.rlim_max,
		"RLIMIT_NOFILE hard changed: {} -> {}",
		before.rlim_max, after.rlim_max
	);
}

#[test]
fn apply_max_files_lowers_soft() {
	// Pick a value strictly below the current soft limit so the test never
	// fails because of system caps. The Go test uses `cur.Cur / 2` with a
	// floor of 16.
	let mut cur = std::mem::MaybeUninit::<libc::rlimit>::uninit();
	let r = unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, cur.as_mut_ptr()) };
	assert_eq!(
		r,
		0,
		"getrlimit failed: {}",
		std::io::Error::last_os_error()
	);
	let cur = unsafe { cur.assume_init() };
	let mut want = cur.rlim_cur / 2;
	if want < 16 {
		want = 16;
	}
	// The chosen value must remain under the hard cap; if the soft cap is
	// already lower than `want`, skip — this matches the Go test's intent
	// ("Pick a value strictly below current hard limit so the test never
	// fails because of system caps.") by skipping when the choice cannot
	// satisfy the assertion.
	if want > cur.rlim_max {
		// Lower the soft toward half of itself until the choice fits.
		want = (cur.rlim_max / 2).max(16);
	}
	if want > cur.rlim_cur || want > cur.rlim_max {
		// Cannot run the assertion — skip via returning early.
		eprintln!(
			"RLIMIT_NOFILE too constrained (cur={}, max={}) — skipping",
			cur.rlim_cur, cur.rlim_max
		);
		return;
	}

	apply(&Limits {
		max_files: want,
		..Default::default()
	})
	.expect("apply MaxFiles");

	let mut got = std::mem::MaybeUninit::<libc::rlimit>::uninit();
	let r = unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, got.as_mut_ptr()) };
	assert_eq!(
		r,
		0,
		"getrlimit failed: {}",
		std::io::Error::last_os_error()
	);
	let got = unsafe { got.assume_init() };

	assert_eq!(
		got.rlim_cur, want,
		"RLIMIT_NOFILE soft: got {} want {}",
		got.rlim_cur, want
	);
	// Soft and hard should match — the Go implementation explicitly sets
	// both to the same value.
	assert_eq!(
		got.rlim_max, want,
		"RLIMIT_NOFILE hard: got {} want {}",
		got.rlim_max, want
	);

	// Restore (best effort — only if we still have headroom).
	let restore = libc::rlimit {
		rlim_cur: cur.rlim_cur,
		rlim_max: cur.rlim_max,
	};
	let _ = unsafe { libc::setrlimit(libc::RLIMIT_NOFILE, &restore) };
}
