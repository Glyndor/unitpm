//! Depth tests for [`super::is_managed_by_package_system`].
//!
//! These don't exercise the apply path — the function isn't currently
//! called from `apply_to_path`. What they do prove is the package-manager
//! probe itself: given a fake `dpkg` on `PATH` that exits 0, the function
//! returns true; given one that exits 1, it returns false; given an
//! empty `PATH`, it returns false. Deleting the function or breaking its
//! logic is caught here.
//!
//! [`is_managed_by_package_system`]: super::is_managed_by_package_system

use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use super::is_managed_by_package_system;

#[allow(unused_imports)]
use crate::updater::is_managed_by_package_system as _;

/// Guard that points `PATH` at a temp directory containing the scripts we
/// want one of the package-manager probes to find. Mutates the process
/// environment, so it has to hold a lock that nothing else in the test
/// suite also touches.
struct PathGuard {
	_held: crate::test_env::Guard,
	prev: Option<String>,
}

impl PathGuard {
	fn new(dir: &Path) -> Self {
		let _held = crate::test_env::lock();
		let prev = std::env::var("PATH").ok();
		std::env::set_var("PATH", dir.as_os_str());
		Self { _held, prev }
	}
}

impl Drop for PathGuard {
	fn drop(&mut self) {
		match self.prev.as_deref() {
			Some(v) => std::env::set_var("PATH", v),
			None => std::env::remove_var("PATH"),
		}
	}
}

fn write_fake_dpkg(dir: &Path, exit_code: i32) {
	let path = dir.join("dpkg");
	std::fs::write(&path, format!("#!/bin/sh\nexit {exit_code}\n")).expect("write");
	let mut perm = std::fs::metadata(&path).expect("meta").permissions();
	perm.set_mode(0o755);
	std::fs::set_permissions(&path, perm).expect("chmod");
}

#[test]
fn is_managed_returns_true_when_dpkg_claims_ownership() {
	let dir = tempfile::tempdir().expect("tempdir");
	write_fake_dpkg(dir.path(), 0);
	let _g = PathGuard::new(dir.path());
	assert!(is_managed_by_package_system());
}

#[test]
fn is_managed_returns_false_when_dpkg_claims_nothing() {
	let dir = tempfile::tempdir().expect("tempdir");
	write_fake_dpkg(dir.path(), 1);
	let _g = PathGuard::new(dir.path());
	// The fake dpkg is on PATH but exits non-zero, so no probe succeeds.
	// The current binary isn't claimed by anything we can fake, so the
	// function must return false.
	assert!(!is_managed_by_package_system());
}

#[test]
fn is_managed_returns_false_when_no_package_tool_in_path() {
	let dir = tempfile::tempdir().expect("tempdir");
	let _g = PathGuard::new(dir.path());
	assert!(!is_managed_by_package_system());
}
