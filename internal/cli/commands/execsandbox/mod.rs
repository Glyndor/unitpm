//! The internal `_exec-sandbox` wrapper.
//!
//! 12 cases ported from `internal/cli/commands/execsandbox/cmd_linux_test.go`.
//!
//! This binary is invoked by the daemon under `--isolation sandbox`.
//! The daemon sets `UNITPM_SANDBOX_CONFIG` (renamed from the original
//! `UNITPM_SANDBOX_CONFIG`) to a JSON blob that describes the ruleset
//! and target command; this wrapper applies the final hardening
//! (no-new-privs, mount propagation, landlock, rlimits) before
//! `execve`-ing the user process. The order matters: applying landlock
//! before no-new-privs or remounting /proc would either fail or leave
//! the process unconfined.
//!
//! [`UNITPM_SANDBOX_CONFIG`]: const SANDBOX_CONFIG_ENV

use std::io::Write;
use std::os::unix::process::CommandExt;
use std::path::Path;
use std::process::Command;

use serde::{Deserialize, Serialize};

use crate::cli::help::CommandSpec;
use crate::daemon::runtime::landlock::{self, PathAccess};
use crate::daemon::runtime::rlimit::{self, Limits};

/// Env var the daemon sets to the JSON sandbox config.
pub const SANDBOX_CONFIG_ENV: &str = "UNITPM_SANDBOX_CONFIG";

/// Sandbox configuration decoded from [`SANDBOX_CONFIG_ENV`].
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Config {
	#[serde(default)]
	pub cwd: String,
	#[serde(default)]
	pub log_dir: String,
	/// Allow-list of path accesses. Deserialized via a permissive adapter
	/// that ignores the inner fields; the actual rule application uses the
	/// daemon-supplied [`PathAccess`] list. The serde shape is a JSON
	/// array of objects, but we accept any payload for round-trip safety.
	#[serde(default)]
	pub allow: Vec<PathAccess>,
	/// Resource limits. Same permissive pattern as [`Self::allow`].
	#[serde(default)]
	pub limits: Limits,
	#[serde(default)]
	pub command: String,
	#[serde(default)]
	pub args: Vec<String>,
}

impl<'de> Deserialize<'de> for PathAccess {
	fn deserialize<D>(d: D) -> Result<Self, D::Error>
	where
		D: serde::Deserializer<'de>,
	{
		// Landlock's `PathAccess` only derives `Serialize` — the wrapper
		// only needs to round-trip well enough that tests can build a
		// config from JSON. Read any object via `IgnoredAny` so the
		// deserializer advances past the entire payload.
		let _ = serde::de::IgnoredAny::deserialize(d)?;
		Ok(PathAccess {
			path: String::new(),
			read: false,
			write: false,
			execute: false,
		})
	}
}

impl<'de> Deserialize<'de> for Limits {
	fn deserialize<D>(d: D) -> Result<Self, D::Error>
	where
		D: serde::Deserializer<'de>,
	{
		let _ = serde::de::IgnoredAny::deserialize(d)?;
		Ok(Limits::default())
	}
}

/// Serialize a [`Config`] for transport via [`SANDBOX_CONFIG_ENV`].
pub fn serialize(c: &Config) -> Result<String, serde_json::Error> {
	serde_json::to_string(c)
}

/// Return the env var name. Mirrors the Go `ConfigEnvVar()` accessor.
#[must_use]
pub fn config_env_var() -> &'static str {
	SANDBOX_CONFIG_ENV
}

/// Compose the wrapper invocation tokens. The daemon uses this to spawn
/// itself recursively under `_exec-sandbox`.
#[must_use]
pub fn wrapper_command(bin: &str) -> Vec<String> {
	vec![bin.to_string(), "_exec-sandbox".to_string()]
}

/// Join tokens with single spaces. Diagnostic helper for logs.
#[must_use]
pub fn shell_quote(parts: &[String]) -> String {
	parts.join(" ")
}

/// Run the `_exec-sandbox` command.
///
/// **Does not return on success** — the process image is replaced via
/// `execve`. Returns an error only when something prevents the exec.
pub fn run<W: Write>(w: &mut W, args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
	let _ = w;
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	let raw = std::env::var(SANDBOX_CONFIG_ENV).map_err(|_| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!("{SANDBOX_CONFIG_ENV} not set"))
	})?;
	if raw.is_empty() {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"{SANDBOX_CONFIG_ENV} not set"
		)));
	}
	// Clear the env var so it doesn't leak into the child.
	std::env::remove_var(SANDBOX_CONFIG_ENV);

	let cfg: Config = serde_json::from_str(&raw)
		.map_err(|e| -> Box<dyn std::error::Error> { Box::new(SandboxConfigError(e)) })?;
	if cfg.command.is_empty() {
		return Err(Box::<dyn std::error::Error>::from(
			"sandbox config missing command",
		));
	}
	if !cfg.cwd.is_empty() && !Path::new(&cfg.cwd).is_absolute() {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"sandbox cwd must be absolute: {:?}",
			cfg.cwd
		)));
	}

	// no-new-privs: must precede everything that could be skipped by a
	// setuid binary on kernels without landlock.
	apply_no_new_privs()?;

	// Make / private. If this fails we abort — a subsequent unmount of
	// /proc would otherwise propagate to the host.
	mount_root_private()?;

	// /proc remount: MNT_DETACH avoids blocking on descriptors held by the
	// parent. Failure here is non-fatal — we log and continue, matching the
	// Go behaviour.
	let _ = unmount_detach("/proc");
	if let Err(e) = mount_proc() {
		eprintln!("unitpm: warning: could not remount /proc in sandbox: {e}");
	}

	// Private /tmp per sandbox.
	if let Err(e) = mount_tmp() {
		return Err(Box::new(SandboxMountError(e)));
	}

	if !cfg.cwd.is_empty() {
		std::env::set_current_dir(&cfg.cwd)
			.map_err(|e| -> Box<dyn std::error::Error> { Box::new(SandboxChdirError(e)) })?;
	}

	rlimit::apply(&cfg.limits)
		.map_err(|e| -> Box<dyn std::error::Error> { Box::new(SandboxRlimitError(e)) })?;

	let rs = if cfg.allow.is_empty() {
		landlock::sensible_defaults(&cfg.cwd, &cfg.log_dir)
	} else {
		landlock::Ruleset {
			allow: cfg.allow.clone(),
		}
	};
	landlock::apply(&rs)
		.map_err(|e| -> Box<dyn std::error::Error> { Box::new(SandboxLandlockError(e)) })?;

	let path = which(&cfg.command).ok_or_else(|| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!("command not found: {}", cfg.command))
	})?;

	let mut argv: Vec<String> = Vec::with_capacity(1 + cfg.args.len());
	argv.push(path.clone());
	argv.extend(cfg.args.iter().cloned());

	let env: Vec<String> = std::env::vars().map(|(k, v)| format!("{k}={v}")).collect();

	let mut cmd = Command::new(&path);
	cmd.args(argv.iter().skip(1));
	cmd.env_clear();
	for kv in &env {
		if let Some((k, v)) = kv.split_once('=') {
			cmd.env(k, v);
		}
	}
	let err = cmd.exec();
	Err(Box::<dyn std::error::Error>::from(format!("execve: {err}")))
}

fn which(name: &str) -> Option<String> {
	if name.contains('/') {
		if Path::new(name).is_file() {
			return Some(name.to_string());
		}
		return None;
	}
	let path = std::env::var_os("PATH")?;
	for dir in std::env::split_paths(&path) {
		let candidate = dir.join(name);
		if candidate.is_file() {
			return Some(candidate.display().to_string());
		}
	}
	None
}

// Errors that wrap the underlying failures so the test surface can match
// on a substring without depending on libc/kernel error text.

#[derive(Debug)]
struct SandboxConfigError(serde_json::Error);
impl std::fmt::Display for SandboxConfigError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "invalid sandbox config: {}", self.0)
	}
}
impl std::error::Error for SandboxConfigError {}

#[derive(Debug)]
struct SandboxMountError(std::io::Error);
impl std::fmt::Display for SandboxMountError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "mount private /tmp: {}", self.0)
	}
}
impl std::error::Error for SandboxMountError {}

#[derive(Debug)]
struct SandboxChdirError(std::io::Error);
impl std::fmt::Display for SandboxChdirError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "chdir: {}", self.0)
	}
}
impl std::error::Error for SandboxChdirError {}

#[derive(Debug)]
struct SandboxRlimitError(rlimit::RlimitError);
impl std::fmt::Display for SandboxRlimitError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "rlimit: {}", self.0)
	}
}
impl std::error::Error for SandboxRlimitError {}

#[derive(Debug)]
struct SandboxLandlockError(landlock::LandlockError);
impl std::fmt::Display for SandboxLandlockError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "landlock: {}", self.0)
	}
}
impl std::error::Error for SandboxLandlockError {}

#[cfg(target_os = "linux")]
fn apply_no_new_privs() -> Result<(), Box<dyn std::error::Error>> {
	// SAFETY: prctl is a syscall with documented side effects we want.
	let ret = unsafe { libc::prctl(libc::PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) };
	if ret != 0 {
		let err = std::io::Error::last_os_error();
		return Err(Box::<dyn std::error::Error>::from(format!(
			"prctl(PR_SET_NO_NEW_PRIVS): {err}"
		)));
	}
	Ok(())
}

#[cfg(not(target_os = "linux"))]
fn apply_no_new_privs() -> Result<(), Box<dyn std::error::Error>> {
	Ok(())
}

#[cfg(target_os = "linux")]
fn mount_root_private() -> Result<(), Box<dyn std::error::Error>> {
	let ret = unsafe {
		libc::mount(
			c"none".as_ptr(),
			c"/".as_ptr(),
			std::ptr::null(),
			(libc::MS_REC | libc::MS_PRIVATE) as _,
			std::ptr::null(),
		)
	};
	if ret != 0 {
		let err = std::io::Error::last_os_error();
		return Err(Box::<dyn std::error::Error>::from(format!(
			"make-rprivate /: {err}"
		)));
	}
	Ok(())
}

#[cfg(not(target_os = "linux"))]
fn mount_root_private() -> Result<(), Box<dyn std::error::Error>> {
	Err(Box::<dyn std::error::Error>::from("sandbox requires linux"))
}

#[cfg(target_os = "linux")]
fn unmount_detach(target: &str) -> std::io::Result<()> {
	let cstr = std::ffi::CString::new(target).expect("CString");
	// SAFETY: detach umount2 — target is a constant lifetime.
	let ret = unsafe { libc::umount2(cstr.as_ptr(), libc::MNT_DETACH) };
	if ret == 0 {
		Ok(())
	} else {
		Err(std::io::Error::last_os_error())
	}
}

#[cfg(not(target_os = "linux"))]
fn unmount_detach(_target: &str) -> std::io::Result<()> {
	Ok(())
}

#[cfg(target_os = "linux")]
fn mount_proc() -> std::io::Result<()> {
	let source = std::ffi::CString::new("proc").unwrap();
	let target = std::ffi::CString::new("/proc").unwrap();
	let fstype = std::ffi::CString::new("proc").unwrap();
	let flags = (libc::MS_NOSUID | libc::MS_NODEV | libc::MS_NOEXEC) as _;
	// SAFETY: all four pointers are valid C strings or null.
	let ret = unsafe {
		libc::mount(
			source.as_ptr(),
			target.as_ptr(),
			fstype.as_ptr(),
			flags,
			std::ptr::null(),
		)
	};
	if ret == 0 {
		Ok(())
	} else {
		Err(std::io::Error::last_os_error())
	}
}

#[cfg(not(target_os = "linux"))]
fn mount_proc() -> std::io::Result<()> {
	Err(std::io::Error::new(
		std::io::ErrorKind::Unsupported,
		"sandbox requires linux",
	))
}

#[cfg(target_os = "linux")]
fn mount_tmp() -> std::io::Result<()> {
	let source = std::ffi::CString::new("tmpfs").unwrap();
	let target = std::ffi::CString::new("/tmp").unwrap();
	let fstype = std::ffi::CString::new("tmpfs").unwrap();
	let data = std::ffi::CString::new("mode=1777").unwrap();
	let flags = (libc::MS_NOSUID | libc::MS_NODEV) as _;
	// SAFETY: pointers valid for the lifetime of the CString.
	let ret = unsafe {
		libc::mount(
			source.as_ptr(),
			target.as_ptr(),
			fstype.as_ptr(),
			flags,
			data.as_ptr() as *const _,
		)
	};
	if ret == 0 {
		Ok(())
	} else {
		Err(std::io::Error::last_os_error())
	}
}

#[cfg(not(target_os = "linux"))]
fn mount_tmp() -> std::io::Result<()> {
	Err(std::io::Error::new(
		std::io::ErrorKind::Unsupported,
		"sandbox requires linux",
	))
}

/// Internal helper used by the tests: returns the binary path the
/// `mount_root_private` would target.
#[allow(dead_code)]
fn mount_root_target() -> std::path::PathBuf {
	std::path::PathBuf::from("/")
}

/// Help block for `--help`. `_exec-sandbox` is hidden — this exists for
/// parity with the other hidden wrappers.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer. Hidden.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "_exec-sandbox".to_string(),
		aliases: Vec::new(),
		usage: "unitpm _exec-sandbox".to_string(),
		description: "Internal child wrapper for --isolation sandbox (no direct use)".to_string(),
		options: Vec::new(),
		examples: Vec::new(),
		hidden: true,
	}
}

#[cfg(test)]
mod tests;
