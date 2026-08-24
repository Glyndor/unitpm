//! Linux-only tests for the paths module.
//!
//! The 20 cases here mirror the three Go test files (`paths_test.go`,
//! `paths_internal_test.go`, `root_internal_test.go`). All exercise state that
//! is Linux-specific (XDG layout, euid, symlink escapes).

use super::*;
use std::path::PathBuf;

// Serialised through the crate-wide lock in `test_env`: the euid override and
// XDG_STATE_HOME are process-global, and so are the variables the transport and
// handler tests write, so a lock private to this module would not exclude them.

/// Holds the lock **and** restores the process-global state on the way out.
///
/// The mutex alone is not enough. It serialises access, but a test that sets
/// the euid override and does not clear it leaves the next test to run under
/// it, so the result depends on the order the runner happens to pick. That is
/// how `resolve_log_paths_default_paths` saw `/var/log/glyndor/unitpm` instead
/// of the XDG path on roughly one run in five.
///
/// Restoring in `Drop` rather than at the end of each test also covers the
/// case that matters most: a failing assertion unwinds, and trailing cleanup
/// never runs at all — so the first failure would poison every later test and
/// hide its own cause.
struct EnvGuard(#[allow(dead_code)] crate::test_env::Guard);

impl Drop for EnvGuard {
	fn drop(&mut self) {
		super::clear_euid_for_tests();
		std::env::remove_var("XDG_STATE_HOME");
	}
}

fn env_lock() -> EnvGuard {
	EnvGuard(crate::test_env::lock())
}

fn temp_xdg(tmp: &tempfile::TempDir) -> PathBuf {
	let dir = tmp.path().to_path_buf();
	std::env::set_var("XDG_STATE_HOME", &dir);
	dir
}

#[test]
fn is_root_with_overrides() {
	let _guard = env_lock();
	set_euid_for_tests(0);
	assert!(is_root());
	set_euid_for_tests(1000);
	assert!(!is_root());
	clear_euid_for_tests();
}

#[test]
fn within_root_table() {
	let cases = [
		(
			"/var/log/glyndor/unitpm",
			"/var/log/glyndor/unitpm/app/stdout.log",
			true,
			"inside",
		),
		(
			"/var/log/glyndor/unitpm",
			"/var/log/glyndor/unitpm",
			true,
			"equal",
		),
		(
			"/var/log/glyndor/unitpm",
			"/var/log/glyndor/passwd",
			false,
			"escape",
		),
		// A sibling whose name *starts with* the root's, which is the case a
		// string-prefix comparison gets wrong and a component-wise one gets
		// right. Without it the table passes against an implementation that
		// only checks `starts_with`, so the two cases above prove less than
		// they look like they do.
		(
			"/var/log/glyndor/unitpm",
			"/var/log/glyndor/unitpm-evil/x.log",
			false,
			"sibling sharing the root's name as a string prefix",
		),
		(
			"/var/log/glyndor/unitpm",
			"/var/log/glyndor/other",
			false,
			"sibling",
		),
	];
	for (root, path, want, name) in cases {
		let got = within_root(Path::new(root), Path::new(path));
		assert_eq!(got, want, "{name}: within_root({root:?}, {path:?})");
	}
}

#[test]
fn resolve_log_paths_default_paths() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	let state = temp_xdg(&tmp);
	let (stdout, stderr) =
		resolve_log_paths("test-proc-id", "", "", "").expect("resolve_log_paths");
	let expected_dir = state.join("unitpm/logs/test-proc-id");
	assert_eq!(stdout.parent(), Some(expected_dir.as_path()));
	assert_eq!(
		stdout.file_name().and_then(|s| s.to_str()),
		Some("stdout.log")
	);
	assert_eq!(
		stderr.file_name().and_then(|s| s.to_str()),
		Some("stderr.log")
	);
}

#[test]
fn resolve_log_paths_custom_dir() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	let state = temp_xdg(&tmp);
	let custom = state.join("myapp/logs");
	let (stdout, stderr) =
		resolve_log_paths("proc-abc", custom.to_str().unwrap(), "", "").expect("resolve_log_paths");
	let expected_base = custom.join("proc-abc");
	assert_eq!(stdout.parent(), Some(expected_base.as_path()));
	assert_eq!(stderr.parent(), Some(expected_base.as_path()));
}

#[test]
fn resolve_log_paths_custom_filenames() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	temp_xdg(&tmp);
	let (stdout, stderr) = resolve_log_paths("proc-1", "", "out.txt", "err.txt").expect("resolve");
	assert_eq!(stdout.file_name().and_then(|s| s.to_str()), Some("out.txt"));
	assert_eq!(stderr.file_name().and_then(|s| s.to_str()), Some("err.txt"));
}

#[test]
fn resolve_log_paths_absolute_custom_filename() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	let state = temp_xdg(&tmp);
	let abs = state.join("custom/out.log");
	let (stdout, _) = resolve_log_paths("proc-1", "", abs.to_str().unwrap(), "").expect("resolve");
	assert_eq!(stdout, abs);
}

#[test]
fn resolve_log_paths_path_too_long() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let long = "a".repeat(5000);
	let err = resolve_log_paths("proc-1", &long, "", "");
	assert!(err.is_err(), "expected error for path too long");
}

#[test]
fn resolve_log_paths_dotdot_dir_rejected() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	temp_xdg(&tmp);
	let err = resolve_log_paths("proc-1", "../escape", "", "");
	assert!(err.is_err(), "expected error for path traversal");
}

#[test]
fn get_log_dir_xdg_state_home() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	let state = temp_xdg(&tmp);
	let dir = get_log_dir("").expect("get_log_dir");
	assert_eq!(dir, state.join("unitpm/logs"));
}

#[test]
fn get_log_dir_fallback_to_home() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	std::env::remove_var("XDG_STATE_HOME");
	let home = match std::env::var("HOME") {
		Ok(h) if !h.is_empty() => PathBuf::from(h),
		_ => {
			// No HOME — skip, mirroring the Go test.
			eprintln!("HOME not set, skipping fallback test");
			return;
		}
	};
	let dir = get_log_dir("").expect("get_log_dir");
	assert_eq!(dir, home.join(".local/state/unitpm/logs"));
}

#[test]
fn get_log_dir_custom_dir() {
	let _guard = env_lock();
	// Pin the uid: these assert the user-mode paths, and dpkg-buildpackage
	// runs the suite as root, where the resolver correctly returns the
	// system paths instead. The guard clears it on the way out.
	set_euid_for_tests(1000);
	let tmp = tempfile::tempdir().expect("tempdir");
	temp_xdg(&tmp);
	let custom = tmp.path().join("mydir");
	let dir = get_log_dir(custom.to_str().unwrap()).expect("get_log_dir");
	assert_eq!(dir, custom);
}

#[test]
fn get_log_dir_root_default() {
	let _guard = env_lock();
	set_euid_for_tests(0);
	let dir = get_log_dir("").expect("get_log_dir");
	assert_eq!(dir, PathBuf::from(LOG_ROOT));
	set_euid_for_tests(1000);
}

#[test]
fn resolve_root_log_dir_not_absolute() {
	let _guard = env_lock();
	set_euid_for_tests(0);
	let err = get_log_dir("relative/path");
	assert!(err.is_err(), "want absolute error");
	let msg = err.unwrap_err().to_string();
	assert!(msg.contains("absolute"), "want absolute error, got {msg}");
}

#[test]
fn resolve_root_log_dir_outside_allowed_roots() {
	let _guard = env_lock();
	set_euid_for_tests(0);
	let err = get_log_dir("/var/log/glyndor/passwd");
	assert!(err.is_err(), "want outside roots error");
	let msg = err.unwrap_err().to_string();
	assert!(
		msg.contains("outside allowed"),
		"want outside roots error, got {msg}"
	);
}

#[test]
fn resolve_root_log_dir_within_xdg_state_home() {
	let _guard = env_lock();
	set_euid_for_tests(0);
	let tmp = tempfile::tempdir().expect("tempdir");
	let state = temp_xdg(&tmp);
	let candidate = state.join("unitpm/logs/sub");
	std::fs::create_dir_all(&candidate).expect("mkdir");
	let got = get_log_dir(candidate.to_str().unwrap()).expect("get_log_dir");
	assert_eq!(got, candidate);
	set_euid_for_tests(1000);
}

#[test]
fn resolve_root_log_dir_nonexistent_inside_root() {
	let _guard = env_lock();
	set_euid_for_tests(0);
	let tmp = tempfile::tempdir().expect("tempdir");
	let state = temp_xdg(&tmp);
	let candidate = state.join("unitpm/logs/does-not-exist");
	let got = get_log_dir(candidate.to_str().unwrap()).expect("get_log_dir");
	assert_eq!(got, candidate);
	set_euid_for_tests(1000);
}

#[test]
fn path_contains_unsafe_symlink_safe() {
	let _guard = env_lock();
	let tmp = tempfile::tempdir().expect("tempdir");
	let root = std::fs::canonicalize(tmp.path()).expect("canonicalize");
	let sub = root.join("a/b");
	std::fs::create_dir_all(&sub).expect("mkdir");
	assert!(!path_contains_unsafe_symlink(&root, &sub));
}

#[test]
fn path_contains_unsafe_symlink_escaping() {
	let _guard = env_lock();
	let tmp = tempfile::tempdir().expect("tempdir");
	let root = std::fs::canonicalize(tmp.path()).expect("canonicalize");
	let outside = tempfile::tempdir().expect("outside tempdir");
	let outside_resolved = std::fs::canonicalize(outside.path()).expect("canonicalize outside");
	let link = root.join("escape");
	std::os::unix::fs::symlink(&outside_resolved, &link).expect("symlink");
	assert!(path_contains_unsafe_symlink(&root, &link.join("x")));
}

#[test]
fn match_resolved_root_nonexistent_safe() {
	let _guard = env_lock();
	let tmp = tempfile::tempdir().expect("tempdir");
	let root = std::fs::canonicalize(tmp.path()).expect("canonicalize");
	let candidate = root.join("fresh");
	assert!(match_resolved_root(&root, &candidate));
}

#[test]
fn match_resolved_root_outside_root() {
	let _guard = env_lock();
	let tmp = tempfile::tempdir().expect("tempdir");
	let root = std::fs::canonicalize(tmp.path()).expect("canonicalize");
	assert!(!match_resolved_root(&root, Path::new("/etc")));
}
