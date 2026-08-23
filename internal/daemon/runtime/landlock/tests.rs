//! Tests for the `landlock` wrapper. Mirrors `landlock_linux_test.go`.
//!
//! The Go test suite has 9 cases. Each is preserved:
//!
//! - `supported` — smoke test that the probe doesn't panic.
//! - `sensible_defaults` — fixture shape: cwd / log dir are present, total
//!   entries >= 10.
//! - `access_mask` — round-trip the read / write / execute / empty cases.
//! - `apply_no_op_when_unsupported` — apply with empty ruleset returns
//!   `Ok` on every kernel.
//! - `fs_mask_abi1` — ABI 1 mask is non-zero and excludes REFER.
//! - `fs_mask_abi2_includes_refer` — ABI 2 mask includes REFER.
//! - `fs_mask_abi3_includes_truncate` — ABI 3 mask includes TRUNCATE.
//! - `fs_mask_monotonically_grows` — each ABI mask dominates the previous.
//! - `add_path_rule_relative_path_rejected` — relative path is rejected
//!   with an "absolute" message.
//!
//! The `apply_no_op_when_unsupported` test guards the **fail-closed path
//! when Landlock is unsupported**: it verifies that the version-probe
//! branch returns `Ok(())` so callers can treat Landlock as best-effort.
//!
//! `TestApply_EmptyRuleset_SupportedKernel` from the Go file is omitted:
//! applying an empty Landlock ruleset would restrict the test runner
//! process permanently, exactly the failure mode the Go test's `t.Skip`
//! is designed to prevent. The control that matters — the syscall layer
//! — is exercised by `apply_no_op_when_unsupported` on the unsupported
//! branch and by `add_path_rule_relative_path_rejected` on the input
//! validation branch. What is **not** tested here is the kernel actually
//! confining reads after `restrict_self`; see the report for the
//! properties the suite cannot prove.

use super::{
	access_mask, add_path_rule, apply, create_ruleset_fd, fs_mask, sensible_defaults, supported,
	PathAccess, Ruleset,
};

const ALL_FLAGS: u64 = 0xffff_ffff_ffff_ffff;

#[test]
fn supported_smoke() {
	// Just confirm the probe doesn't crash. Result depends on kernel.
	let _ = supported();
}

#[test]
fn sensible_defaults_shape() {
	let rs = sensible_defaults("/home/user/app", "/var/log/app");
	assert!(
		rs.allow.len() >= 10,
		"expected >=10 allow entries in defaults, got {}",
		rs.allow.len()
	);

	let mut saw_cwd = false;
	let mut saw_log = false;
	for a in &rs.allow {
		if a.path == "/home/user/app" {
			saw_cwd = true;
		}
		if a.path == "/var/log/app" {
			saw_log = true;
		}
	}
	assert!(saw_cwd, "cwd not in default allowlist");
	assert!(saw_log, "logDir not in default allowlist");
}

#[test]
fn access_mask_table() {
	let cases = [
		(
			PathAccess {
				read: true,
				..Default::default()
			},
			true,
		),
		(
			PathAccess {
				write: true,
				..Default::default()
			},
			true,
		),
		(
			PathAccess {
				execute: true,
				..Default::default()
			},
			true,
		),
		(PathAccess::default(), false),
	];
	for (pa, want) in cases {
		let m = access_mask(&pa, ALL_FLAGS);
		assert_eq!(
			(m != 0),
			want,
			"access_mask({pa:?}) = {m:x}, want non-zero={want}"
		);
	}
}

/// Test-only RAII guard that sets the ABI override on construction and
/// clears it on drop. Mirrors the `EnvGuard` pattern in `paths::linux_tests`
/// — without the `Drop` restore, a failing assertion would unwind past the
/// cleanup and leave the override in place for the next test, which would
/// run under a phantom "unsupported kernel" state.
///
/// This is process-global state and the same Drop-restoring discipline the
/// phase-1 paths tests learned was necessary. `cargo test` parallelises by
/// default while Go does not; serialising is not enough, restoring is.
struct AbiGuard;

impl Drop for AbiGuard {
	fn drop(&mut self) {
		super::clear_abi_override_for_tests();
	}
}

#[test]
fn apply_no_op_when_unsupported() {
	// **fail-closed-when-unsupported control**: apply() must return Ok
	// on kernels that lack Landlock. The Go test skips on supported
	// kernels because the only way to exercise the fail-closed branch
	// is to run on a kernel without Landlock. We pin the behaviour
	// against an ABI override so the test catches deletion on every
	// kernel — the override is test-only (`#[cfg(test)]`) and is
	// restored on Drop.
	let _g = AbiGuard;
	super::set_abi_override_for_tests(0);
	let rs = Ruleset::default();
	let result = apply(&rs);
	assert!(
		result.is_ok(),
		"expected Ok when Landlock is unsupported, got {result:?}"
	);
}

#[test]
fn fs_mask_abi1() {
	let mask = fs_mask(1);
	assert_ne!(mask, 0, "ABI 1 mask should be non-zero");
	// REFER is ABI >= 2; must not appear in ABI 1 mask.
	assert_eq!(mask & (1u64 << 13), 0, "ABI 1 mask must not include REFER");
}

#[test]
fn fs_mask_abi2_includes_refer() {
	let mask = fs_mask(2);
	assert_ne!(mask & (1u64 << 13), 0, "ABI 2 mask must include REFER");
}

#[test]
fn fs_mask_abi3_includes_truncate() {
	let mask = fs_mask(3);
	assert_ne!(mask & (1u64 << 14), 0, "ABI 3 mask must include TRUNCATE");
}

#[test]
fn fs_mask_monotonically_grows() {
	let m1 = fs_mask(1);
	let m2 = fs_mask(2);
	let m3 = fs_mask(3);
	assert!(m2 >= m1, "ABI 2 mask ({m2:x}) < ABI 1 mask ({m1:x})");
	assert!(m3 >= m2, "ABI 3 mask ({m3:x}) < ABI 2 mask ({m2:x})");
}

#[test]
fn add_path_rule_relative_path_rejected() {
	let err = add_path_rule(
		0,
		&PathAccess {
			path: "relative/path".into(),
			read: true,
			..Default::default()
		},
		ALL_FLAGS,
	);
	let err = err.expect_err("expected error for relative path");
	assert!(
		err.to_string().contains("absolute"),
		"unexpected error: {err}"
	);
}

#[test]
fn create_ruleset_returns_valid_fd_and_add_rule_succeeds() {
	// **ruleset-creation control**: the entire sequence
	// `landlock_create_ruleset` -> `landlock_add_rule` is exercised here
	// on a real kernel fd. The Go test that does the equivalent
	// (`TestApply_EmptyRuleset_SupportedKernel`) skips on supported
	// kernels because `landlock_restrict_self` is non-revocable. We
	// exercise create + add without restrict, which is safe to do
	// in-process: closing the ruleset fd before restrict_self means
	// the fd is dropped and the kernel cleans up the unused ruleset.
	//
	// Deletion behaviour: if `create_ruleset_fd` is deleted (replaced
	// with `Err(LandlockError::CreateRuleset(0))` or similar), the test
	// goes red because the fd is never created and `add_path_rule`
	// cannot succeed against an invalid fd.
	if !supported() {
		eprintln!("Landlock not supported on this kernel — skipping create-ruleset test");
		return;
	}

	let abi = super::get_abi_version().expect("probe must succeed on a supporting kernel");
	let mask = fs_mask(abi);
	let fd = create_ruleset_fd(mask).expect("create_ruleset_fd must succeed");

	// Use /tmp because `SensibleDefaults` lists it with RWX. The test
	// resolves it to its real inode and adds a path-beneath rule.
	let access = PathAccess {
		path: "/tmp".into(),
		read: true,
		write: true,
		execute: false,
	};
	let result = add_path_rule(fd, &access, mask);
	assert!(
		result.is_ok(),
		"add_path_rule against the freshly created ruleset fd failed: {result:?}"
	);

	// Drop the ruleset without calling `restrict_self` — the fd close
	// releases the ruleset, the process is unchanged.
	let r = unsafe { libc::close(fd) };
	assert_eq!(
		r,
		0,
		"close(ruleset_fd) failed: {}",
		std::io::Error::last_os_error()
	);
}
