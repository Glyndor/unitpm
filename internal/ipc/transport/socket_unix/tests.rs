//! Tests for [`crate::ipc::transport::socket_unix::get_socket_path`].
//!
//! Mirrored from `socket_unix_test.go`. Three of the cases skip when the
//! test runs as root: the user-mode defaults would otherwise resolve to the
//! system socket and the test would not exercise what it claims to.
//!
//! These tests mutate process-global environment variables
//! (`UNITPM_SOCKET`, `XDG_RUNTIME_DIR`). `EnvGuard` serialises access and
//! restores the original values on the way out, both for cleanliness and
//! because a failing assertion that unwinds without `Drop` running would
//! otherwise leak state into whatever test runs next.

use std::os::unix::fs::PermissionsExt;

use crate::ipc::transport::socket_unix::{get_socket_path, SocketPathError};

struct EnvGuard {
	_unit: crate::test_env::Guard,
	saved_socket: Option<String>,
	saved_xdg: Option<String>,
}

impl EnvGuard {
	fn new() -> Self {
		let saved_socket = std::env::var("UNITPM_SOCKET").ok();
		let saved_xdg = std::env::var("XDG_RUNTIME_DIR").ok();
		let _unit = crate::test_env::lock();
		Self {
			_unit,
			saved_socket,
			saved_xdg,
		}
	}
}

impl Drop for EnvGuard {
	fn drop(&mut self) {
		match &self.saved_socket {
			Some(v) => std::env::set_var("UNITPM_SOCKET", v),
			None => std::env::remove_var("UNITPM_SOCKET"),
		}
		match &self.saved_xdg {
			Some(v) => std::env::set_var("XDG_RUNTIME_DIR", v),
			None => std::env::remove_var("XDG_RUNTIME_DIR"),
		}
	}
}

fn root_uid() -> u32 {
	unsafe { libc::geteuid() }
}

#[test]
fn get_socket_path_absolute_env_override() {
	let _g = EnvGuard::new();
	let dir = tempfile::tempdir().expect("tempdir");
	let sock_path = dir.path().join("test.sock");
	std::env::set_var("UNITPM_SOCKET", sock_path.as_os_str());
	let got = get_socket_path().expect("absolute override");
	assert_eq!(got, sock_path.display().to_string());
}

#[test]
fn get_socket_path_relative_path_rejected() {
	let _g = EnvGuard::new();
	std::env::set_var("UNITPM_SOCKET", "relative/path/unitpm.sock");
	let err = get_socket_path();
	let err = err.expect_err("expected error for relative UNITPM_SOCKET path");
	let msg = err.to_string();
	assert!(
		msg.contains("absolute"),
		"error should mention absolute, got: {msg}"
	);
}

#[test]
fn get_socket_path_world_writable_parent_rejected() {
	let _g = EnvGuard::new();
	let dir = tempfile::tempdir().expect("tempdir");
	let p = dir.path();
	std::fs::set_permissions(p, std::fs::Permissions::from_mode(0o777)).expect("chmod");
	std::env::set_var("UNITPM_SOCKET", p.join("unitpm.sock"));
	let err = get_socket_path();
	let err = err.expect_err("expected error for world-writable parent");
	let msg = err.to_string();
	assert!(
		msg.contains("world-writable"),
		"error should mention world-writable, got: {msg}"
	);
}

#[test]
fn get_socket_path_missing_xdg_runtime_dir() {
	let _g = EnvGuard::new();
	if root_uid() == 0 {
		eprintln!("running as root; XDG_RUNTIME_DIR check is bypassed");
		return;
	}
	std::env::set_var("UNITPM_SOCKET", "");
	std::env::remove_var("XDG_RUNTIME_DIR");
	let err = get_socket_path().expect_err("expected error when XDG_RUNTIME_DIR is unset");
	let msg = err.to_string();
	assert!(
		msg.contains("XDG_RUNTIME_DIR"),
		"error should mention XDG_RUNTIME_DIR, got: {msg}"
	);
}

#[test]
fn get_socket_path_xdg_runtime_dir_used() {
	let _g = EnvGuard::new();
	if root_uid() == 0 {
		eprintln!("running as root; uses fixed /run/unitpmd/unitpm.sock instead");
		return;
	}
	let dir = tempfile::tempdir().expect("tempdir");
	std::env::set_var("UNITPM_SOCKET", "");
	std::env::set_var("XDG_RUNTIME_DIR", dir.path());
	let got = get_socket_path().expect("XDG path resolves");
	assert!(
		got.starts_with(dir.path().to_str().unwrap()),
		"socket path {got:?} should be under XDG_RUNTIME_DIR {:?}",
		dir.path(),
	);
	assert!(
		got.ends_with("unitpm.sock"),
		"socket path should end with unitpm.sock, got: {got}"
	);
}

#[test]
fn get_socket_path_env_override_precedes_xdg() {
	let _g = EnvGuard::new();
	let dir = tempfile::tempdir().expect("tempdir");
	let explicit = dir.path().join("explicit.sock");
	let xdg_dir = tempfile::tempdir().expect("tempdir");
	std::env::set_var("UNITPM_SOCKET", &explicit);
	std::env::set_var("XDG_RUNTIME_DIR", xdg_dir.path());
	let got = get_socket_path().expect("env override wins");
	assert_eq!(
		got,
		explicit.display().to_string(),
		"UNITPM_SOCKET override must win"
	);
}

#[test]
fn get_socket_path_daemon_unreachable_error_connection_refused() {
	let _g = EnvGuard::new();
	let dir = tempfile::tempdir().expect("tempdir");
	let sock_path = dir.path().join("nope.sock");
	std::env::set_var("UNITPM_SOCKET", &sock_path);
	std::env::set_var("XDG_RUNTIME_DIR", dir.path());

	let err = match crate::ipc::transport::Client::new() {
		Ok(_) => panic!("expected error when daemon not running, got Ok"),
		Err(e) => e,
	};
	let msg = err.to_string();
	assert!(
		msg.contains("cannot reach") || msg.contains("unitpm"),
		"error message not user-friendly: {msg}"
	);
}

#[test]
fn get_socket_path_daemon_unreachable_user_mode_hint() {
	let _g = EnvGuard::new();
	let dir = tempfile::tempdir().expect("tempdir");
	let sock_path = dir
		.path()
		.join("run")
		.join("user")
		.join("1000")
		.join("unitpm.sock");
	std::fs::create_dir_all(sock_path.parent().unwrap()).expect("mkdir");
	std::fs::set_permissions(
		sock_path.parent().unwrap(),
		std::fs::Permissions::from_mode(0o700),
	)
	.expect("chmod");
	std::env::set_var("UNITPM_SOCKET", &sock_path);
	std::env::set_var(
		"XDG_RUNTIME_DIR",
		dir.path().join("run").join("user").join("1000"),
	);

	let err = match crate::ipc::transport::Client::new() {
		Ok(_) => panic!("expected error"),
		Err(e) => e,
	};
	let msg = err.to_string();
	assert!(
		msg.contains("unitpm"),
		"user-mode error should mention unitpm: {msg}"
	);
}

#[test]
fn socket_path_error_displays_useful_message() {
	let e = SocketPathError::RelativeEnv("foo".into());
	assert!(e.to_string().contains("absolute"));
}
