//! Tests for `wrap_sandbox`. Mirrors `sandbox_linux_test.go`.
//!
//! Seven cases are preserved:
//!
//! - `wrap_sandbox_empty_binary_rejected`
//! - `wrap_sandbox_wrapper_path`
//! - `wrap_sandbox_config_env_var_set`
//! - `wrap_sandbox_io_propagated`
//! - `wrap_sandbox_namespace_flags`
//! - `wrap_sandbox_uid_mapped_to_current`
//! - `wrap_sandbox_allow_list_encoded`
//! - `wrap_sandbox_no_error_regardless_of_landlock_support`
//!
//! The Go test propagates stdio by interface-pointer equality. The Rust
//! port uses `Arc<dyn Read/Write>` and verifies propagation via
//! `Arc::ptr_eq`. The semantics are the same: the stream I passed in is
//! the stream the wrapped command carries.

use std::io::{Cursor, Read, Write};
use std::sync::Arc;

use crate::daemon::runtime::landlock::PathAccess;
use crate::daemon::runtime::rlimit::Limits;
use crate::daemon::runtime::sandbox::{
	wrap_sandbox, CommandLike, SandboxOptions, CONFIG_ENV_VAR, WRAPPER_SUBCOMMAND,
};

fn cmd_simple(path: &str, args: &[&str]) -> CommandLike {
	CommandLike {
		path: path.to_string(),
		args: args.iter().map(|s| s.to_string()).collect(),
		env: Vec::new(),
		stdin: None,
		stdout: None,
		stderr: None,
	}
}

#[test]
fn wrap_sandbox_empty_binary_rejected() {
	let cmd = cmd_simple("/bin/true", &[]);
	let err = wrap_sandbox(&cmd, &SandboxOptions::default())
		.expect_err("expected error for empty binary");
	assert!(err.to_string().contains("binary not set"), "got: {err}");
}

#[test]
fn wrap_sandbox_wrapper_path() {
	let cmd = cmd_simple("/bin/echo", &["hello"]);
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		cwd: "/tmp".into(),
		..Default::default()
	};
	let wrapped = wrap_sandbox(&cmd, &opts).expect("wrap_sandbox");

	assert_eq!(wrapped.binary, "/usr/bin/unitpm");
	assert!(
		wrapped.args.len() >= 2,
		"wrapped.args.len() = {}, want >= 2",
		wrapped.args.len()
	);
	assert_eq!(
		wrapped.args[1], WRAPPER_SUBCOMMAND,
		"second arg should be the wrapper subcommand"
	);
}

#[test]
fn wrap_sandbox_config_env_var_set() {
	let cmd = CommandLike {
		path: "/bin/true".into(),
		env: vec!["EXISTING=var".into()],
		..Default::default()
	};
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		cwd: "/tmp".into(),
		log_dir: "/var/log/glyndor/unitpm".into(),
		limits: Limits::default(),
		..Default::default()
	};
	let wrapped = wrap_sandbox(&cmd, &opts).expect("wrap_sandbox");

	let mut config_env = None;
	let mut found_existing = false;
	for e in &wrapped.env {
		if let Some(payload) = e.strip_prefix(&format!("{CONFIG_ENV_VAR}=")) {
			config_env = Some(payload.to_string());
		}
		if e == "EXISTING=var" {
			found_existing = true;
		}
	}
	let payload = config_env.expect("CONFIG_ENV_VAR not found in wrapped env");
	assert!(
		payload.contains("\"cwd\":\"/tmp\""),
		"config payload missing cwd: {payload}"
	);
	assert!(
		payload.contains("\"command\":\"/bin/true\""),
		"config payload missing command: {payload}"
	);
	assert!(found_existing, "original env not preserved in wrapped cmd");
}

#[test]
fn wrap_sandbox_io_propagated() {
	let stdout = Arc::new(Cursor::new(Vec::<u8>::new())) as Arc<dyn Write + Send + Sync>;
	let stderr_in: Arc<dyn Write + Send + Sync> =
		Arc::new(Cursor::new(Vec::<u8>::new())) as Arc<dyn Write + Send + Sync>;
	let stdin_in: Arc<dyn Read + Send + Sync> =
		Arc::new(Cursor::new(Vec::<u8>::new())) as Arc<dyn Read + Send + Sync>;

	let cmd = CommandLike {
		path: "/bin/true".into(),
		stdin: Some(stdin_in.clone()),
		stdout: Some(stdout.clone()),
		stderr: Some(stderr_in.clone()),
		..Default::default()
	};
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		..Default::default()
	};
	let wrapped = wrap_sandbox(&cmd, &opts).expect("wrap_sandbox");

	// Propagated streams must be the same instance we passed in. The Go
	// test uses interface-pointer equality; the Rust equivalent is
	// `Arc::ptr_eq` (the underlying allocation is the same).
	assert!(
		Arc::ptr_eq(wrapped.stdin.as_ref().expect("stdin"), &stdin_in),
		"stdin not propagated"
	);
	assert!(
		Arc::ptr_eq(wrapped.stdout.as_ref().expect("stdout"), &stdout),
		"stdout not propagated"
	);
	assert!(
		Arc::ptr_eq(wrapped.stderr.as_ref().expect("stderr"), &stderr_in),
		"stderr not propagated"
	);
}

#[test]
fn wrap_sandbox_namespace_flags() {
	let cmd = cmd_simple("/bin/true", &[]);
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		..Default::default()
	};
	let wrapped = wrap_sandbox(&cmd, &opts).expect("wrap_sandbox");

	let want = libc::CLONE_NEWUSER | libc::CLONE_NEWPID | libc::CLONE_NEWNS;
	assert_eq!(
		wrapped.sys_proc_attr.clone_flags as i32, want,
		"clone_flags = {:#x}, want {:#x}",
		wrapped.sys_proc_attr.clone_flags, want
	);
	assert!(
		!wrapped.sys_proc_attr.gid_mappings_enable_setgroups,
		"GidMappingsEnableSetgroups must be false to prevent privilege escalation"
	);
	assert!(
		wrapped.sys_proc_attr.set_pgid,
		"set_pgid should be true for process group isolation"
	);
}

#[test]
fn wrap_sandbox_uid_mapped_to_current() {
	let cmd = cmd_simple("/bin/true", &[]);
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		..Default::default()
	};
	let wrapped = wrap_sandbox(&cmd, &opts).expect("wrap_sandbox");

	let uid = unsafe { libc::geteuid() };
	let gid = unsafe { libc::getegid() };

	let uid_mappings = &wrapped.sys_proc_attr.uid_mappings;
	assert_eq!(
		uid_mappings.len(),
		1,
		"UidMappings len = {}, want 1",
		uid_mappings.len()
	);
	assert_eq!(
		uid_mappings[0].container_id, 0,
		"UidMappings ContainerID = {}, want 0",
		uid_mappings[0].container_id
	);
	assert_eq!(
		uid_mappings[0].host_id, uid as u32,
		"UidMappings HostID = {}, want {}",
		uid_mappings[0].host_id, uid
	);
	assert_eq!(
		uid_mappings[0].size, 1,
		"UidMappings Size = {}, want 1",
		uid_mappings[0].size
	);

	let gid_mappings = &wrapped.sys_proc_attr.gid_mappings;
	assert_eq!(
		gid_mappings.len(),
		1,
		"GidMappings len = {}, want 1",
		gid_mappings.len()
	);
	assert_eq!(
		gid_mappings[0].container_id, 0,
		"GidMappings ContainerID = {}, want 0",
		gid_mappings[0].container_id
	);
	assert_eq!(
		gid_mappings[0].host_id, gid as u32,
		"GidMappings HostID = {}, want {}",
		gid_mappings[0].host_id, gid
	);
	assert_eq!(
		gid_mappings[0].size, 1,
		"GidMappings Size = {}, want 1",
		gid_mappings[0].size
	);
}

#[test]
fn wrap_sandbox_allow_list_encoded() {
	let cmd = cmd_simple("/bin/true", &[]);
	let allow = vec![PathAccess {
		path: "/srv/app".into(),
		read: true,
		execute: true,
		..Default::default()
	}];
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		allow,
		..Default::default()
	};
	let wrapped = wrap_sandbox(&cmd, &opts).expect("wrap_sandbox");

	let mut found_payload = false;
	for e in &wrapped.env {
		if let Some(payload) = e.strip_prefix(&format!("{CONFIG_ENV_VAR}=")) {
			found_payload = true;
			assert!(
				payload.contains("/srv/app"),
				"allow path /srv/app not in config: {payload}"
			);
		}
	}
	assert!(found_payload, "{CONFIG_ENV_VAR} not found in env");
}

#[test]
fn wrap_sandbox_no_error_regardless_of_landlock_support() {
	// `wrap_sandbox` must succeed even when Landlock is unsupported — it
	// only prints a warning. This verifies we never return an error for
	// that path.
	let cmd = cmd_simple("/bin/true", &[]);
	let opts = SandboxOptions {
		binary: "/usr/bin/unitpm".into(),
		..Default::default()
	};
	let res = wrap_sandbox(&cmd, &opts);
	assert!(
		res.is_ok(),
		"wrap_sandbox should not error regardless of Landlock support: {:?}",
		res.err()
	);
}
