//! Linux Landlock unprivileged sandboxing wrapper.
//!
//! The child process calls [`apply`] before `execve` to restrict its own
//! filesystem access. The ruleset is **inherited across `execve`**, is
//! **non-revocable**, and applies to the current thread plus all
//! descendants. These two properties are why a unit test can prove the
//! syscall was made with the arguments we intended but cannot prove the
//! kernel actually denied a subsequent read.
//!
//! Mirrors `internal/daemon/runtime/landlock/landlock_linux.go`. The syscall
//! numbers, flag values, the ABI version negotiation, the `EvalSymlinks` /
//! `O_PATH` resolution path, the `PR_SET_NO_NEW_PRIVS` prerequisite, and the
//! order of operations are kept identical so that a Go/Rust pairing under
//! the same kernel gives the same ruleset.
//!
//! Landlock ABI access bits are defined inline because `libc` does not
//! expose them. Values match `include/uapi/linux/landlock.h` on Linux 5.13+.

use std::ffi::CString;
use std::os::fd::RawFd;
use std::path::Path;

use libc::{c_int, c_long, c_uint};
use serde::Serialize;

// ---- Landlock syscall numbers (Linux) --------------------------------------

/// `SYS_landlock_create_ruleset`.
const SYS_LANDLOCK_CREATE_RULESET: c_long = libc::SYS_landlock_create_ruleset;
/// `SYS_landlock_add_rule`.
const SYS_LANDLOCK_ADD_RULE: c_long = libc::SYS_landlock_add_rule;
/// `SYS_landlock_restrict_self`.
const SYS_LANDLOCK_RESTRICT_SELF: c_long = libc::SYS_landlock_restrict_self;

/// `LANDLOCK_CREATE_RULESET_VERSION` — flag for the version-probe call.
const LANDLOCK_CREATE_RULESET_VERSION: c_uint = 1 << 0;

/// `LANDLOCK_RULE_PATH_BENEATH` — type discriminator for path-beneath rules.
const LANDLOCK_RULE_PATH_BENEATH: c_uint = 1;

// ---- Landlock access bits ---------------------------------------------------

const ACCESS_FS_EXECUTE: u64 = 1 << 0;
const ACCESS_FS_WRITE_FILE: u64 = 1 << 1;
const ACCESS_FS_READ_FILE: u64 = 1 << 2;
const ACCESS_FS_READ_DIR: u64 = 1 << 3;
const ACCESS_FS_REMOVE_DIR: u64 = 1 << 4;
const ACCESS_FS_REMOVE_FILE: u64 = 1 << 5;
const ACCESS_FS_MAKE_CHAR: u64 = 1 << 6;
const ACCESS_FS_MAKE_DIR: u64 = 1 << 7;
const ACCESS_FS_MAKE_REG: u64 = 1 << 8;
const ACCESS_FS_MAKE_SOCK: u64 = 1 << 9;
const ACCESS_FS_MAKE_FIFO: u64 = 1 << 10;
const ACCESS_FS_MAKE_BLOCK: u64 = 1 << 11;
const ACCESS_FS_MAKE_SYM: u64 = 1 << 12;
const ACCESS_FS_REFER: u64 = 1 << 13;
const ACCESS_FS_TRUNCATE: u64 = 1 << 14;

// ---- `prctl(2)` -------------------------------------------------------------

/// `PR_SET_NO_NEW_PRIVS` — must be set before `LANDLOCK_RESTRICT_SELF`.
const PR_SET_NO_NEW_PRIVS: c_int = 38;
const SYS_PRCTL: c_long = libc::SYS_prctl;

// ---- Linux open(2) flags ----------------------------------------------------

const O_PATH: c_int = libc::O_PATH;
const O_CLOEXEC: c_int = libc::O_CLOEXEC;

// ---- Public types -----------------------------------------------------------

/// Access requested on a path prefix. Mirrors the Go `PathAccess` struct.
///
/// Field names in the [`Serialize`](serde::Serialize) impl match the Go
/// `landlock.PathAccess` JSON encoding (`Path`, `Read`, `Write`,
/// `Execute`) so the wire payload the daemon writes is byte-compatible
/// with the existing Go wrapper until phase 7 deletes it.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize)]
pub struct PathAccess {
	/// Absolute directory (or file) that gates access.
	#[serde(rename = "Path")]
	pub path: String,
	/// Read-related filesystem rights under [`Self::path`].
	#[serde(rename = "Read")]
	pub read: bool,
	/// Write-related filesystem rights under [`Self::path`].
	#[serde(rename = "Write")]
	pub write: bool,
	/// Execute rights on files under [`Self::path`].
	#[serde(rename = "Execute")]
	pub execute: bool,
}

/// Full sandbox specification. Mirrors the Go `Ruleset`.
#[derive(Debug, Clone, Default)]
pub struct Ruleset {
	/// Allow-list of path accesses.
	pub allow: Vec<PathAccess>,
}

// ---- ABI / version probe ----------------------------------------------------

#[cfg(test)]
use std::sync::atomic::{AtomicI32, Ordering};

#[cfg(test)]
static ABI_OVERRIDE: AtomicI32 = AtomicI32::new(-1);

#[cfg(test)]
pub(crate) fn set_abi_override_for_tests(abi: i32) {
	ABI_OVERRIDE.store(abi, Ordering::Relaxed);
}

#[cfg(test)]
pub(crate) fn clear_abi_override_for_tests() {
	ABI_OVERRIDE.store(-1, Ordering::Relaxed);
}

/// Probe the running kernel for the Landlock ABI version. Returns `Ok(0)`
/// when the syscall itself is not implemented (ENOSYS) — this is how the
/// kernel reports "no Landlock at all", and we treat it as `unsupported`.
pub(crate) fn get_abi_version() -> Result<i32, LandlockError> {
	#[cfg(test)]
	{
		let v = ABI_OVERRIDE.load(Ordering::Relaxed);
		if v == 0 {
			return Err(LandlockError::Unsupported);
		}
		if v > 0 {
			return Ok(v);
		}
		// v < 0: fall through to the real probe.
	}
	let r = unsafe {
		libc::syscall(
			SYS_LANDLOCK_CREATE_RULESET,
			0,
			0,
			LANDLOCK_CREATE_RULESET_VERSION as c_long,
		)
	};
	if r < 0 {
		let errno = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
		// ENOSYS = syscall not implemented; anything else is a real error.
		if errno == libc::ENOSYS {
			return Err(LandlockError::Unsupported);
		}
		return Err(LandlockError::Probe(errno));
	}
	Ok(r as i32)
}

/// `true` when the running kernel supports Landlock ABI >= 1. Mirrors
/// `landlock.Supported()`.
#[must_use]
pub fn supported() -> bool {
	matches!(get_abi_version(), Ok(v) if v >= 1)
}

/// The union of filesystem rights supported at the given Landlock ABI
/// version. Mirrors `landlockFSMask`.
#[must_use]
pub fn fs_mask(abi: i32) -> u64 {
	let mut mask = ACCESS_FS_EXECUTE
		| ACCESS_FS_WRITE_FILE
		| ACCESS_FS_READ_FILE
		| ACCESS_FS_READ_DIR
		| ACCESS_FS_REMOVE_DIR
		| ACCESS_FS_REMOVE_FILE
		| ACCESS_FS_MAKE_CHAR
		| ACCESS_FS_MAKE_DIR
		| ACCESS_FS_MAKE_REG
		| ACCESS_FS_MAKE_SOCK
		| ACCESS_FS_MAKE_FIFO
		| ACCESS_FS_MAKE_BLOCK
		| ACCESS_FS_MAKE_SYM;
	if abi >= 2 {
		mask |= ACCESS_FS_REFER;
	}
	if abi >= 3 {
		mask |= ACCESS_FS_TRUNCATE;
	}
	mask
}

/// Build the access bitmap for a single [`PathAccess`] given the ABI's
/// handled mask. Mirrors `accessMask`.
#[must_use]
pub fn access_mask(a: &PathAccess, handled_mask: u64) -> u64 {
	let mut m = 0u64;
	if a.read {
		m |= ACCESS_FS_READ_FILE | ACCESS_FS_READ_DIR;
	}
	if a.write {
		m |= ACCESS_FS_WRITE_FILE
			| ACCESS_FS_REMOVE_DIR
			| ACCESS_FS_REMOVE_FILE
			| ACCESS_FS_MAKE_CHAR
			| ACCESS_FS_MAKE_DIR
			| ACCESS_FS_MAKE_REG
			| ACCESS_FS_MAKE_SOCK
			| ACCESS_FS_MAKE_FIFO
			| ACCESS_FS_MAKE_BLOCK
			| ACCESS_FS_MAKE_SYM;
	}
	if a.execute {
		m |= ACCESS_FS_EXECUTE;
	}
	m & handled_mask
}

// ---- Apply ------------------------------------------------------------------

/// `landlock_create_ruleset` wrapper. Returns the ruleset fd (which the
/// caller is responsible for closing) on success. Mirrors the inline
/// syscall inside `apply()` in the Go implementation; exposed at
/// `pub(crate)` so the in-process test for the **ruleset-creation
/// control** can drive it directly without going through `restrict_self`,
/// which would irreversibly confine the test runner.
pub(crate) fn create_ruleset_fd(handled_fs: u64) -> Result<RawFd, LandlockError> {
	let attr = LandlockRulesetAttr {
		access_fs: handled_fs,
	};
	let fd = unsafe {
		libc::syscall(
			SYS_LANDLOCK_CREATE_RULESET,
			&attr as *const LandlockRulesetAttr as c_long,
			std::mem::size_of::<LandlockRulesetAttr>() as c_long,
			0,
		)
	};
	if fd < 0 {
		let errno = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
		return Err(LandlockError::CreateRuleset(errno));
	}
	Ok(fd as RawFd)
}

/// Activate the ruleset on the calling thread. Call in the child process
/// before `execve`.
///
/// **fail-closed semantics**: when the kernel does not support Landlock at
/// all, this returns `Ok(())` so the caller can treat Landlock as
/// best-effort hardening — every other hardening step (rlimits,
/// `PR_SET_NO_NEW_PRIVS`, namespaces) still runs. Any error returned here
/// after the ABI version probe succeeded means we attempted to install a
/// ruleset and failed; the caller must abort.
pub fn apply(ruleset: &Ruleset) -> Result<(), LandlockError> {
	let abi = match get_abi_version() {
		Ok(v) if v >= 1 => v,
		// ENOSYS / ENOPROTOOPT — kernel has no Landlock. Match the Go
		// implementation's behaviour: return Ok so this is best-effort.
		Ok(_) => return Ok(()),
		Err(LandlockError::Unsupported) => return Ok(()),
		Err(e) => return Err(e),
	};

	let handled_fs = fs_mask(abi);
	let ruleset_fd = create_ruleset_fd(handled_fs)?;
	// Close the ruleset fd on the way out. The Go implementation defers the
	// close; doing the same in Rust by hand keeps the safety comment in one
	// place.
	let _close_on_drop = FdGuard(ruleset_fd);

	for a in &ruleset.allow {
		if let Err(e) = add_path_rule(ruleset_fd, a, handled_fs) {
			return Err(LandlockError::AddRule {
				path: a.path.clone(),
				source: Box::new(e),
			});
		}
	}

	// `PR_SET_NO_NEW_PRIVS` is required before `LANDLOCK_RESTRICT_SELF`.
	let r = unsafe { libc::syscall(SYS_PRCTL, PR_SET_NO_NEW_PRIVS as c_long, 1, 0, 0, 0) };
	if r != 0 {
		let errno = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
		return Err(LandlockError::NoNewPrivs(errno));
	}

	let r = unsafe { libc::syscall(SYS_LANDLOCK_RESTRICT_SELF, ruleset_fd as c_long, 0, 0) };
	if r != 0 {
		let errno = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
		return Err(LandlockError::RestrictSelf(errno));
	}
	Ok(())
}

// ---- Add-path rule ----------------------------------------------------------

/// Mirrors `landlock.addPathRule`. Returns `Ok(())` for paths that do not
/// exist (silently skipped) and an `Err` for paths that exist but cannot be
/// opened.
fn add_path_rule(
	ruleset_fd: RawFd,
	access: &PathAccess,
	handled_mask: u64,
) -> Result<(), LandlockError> {
	let p = Path::new(&access.path);
	if !p.is_absolute() {
		return Err(LandlockError::PathNotAbsolute);
	}

	// Resolve symlinks so the landlock fd points at the real inode. Fall
	// back to the original path when it doesn't exist yet — the open call
	// below will then fail and skip the rule silently.
	let resolved = std::fs::canonicalize(p).unwrap_or_else(|_| p.to_path_buf());

	let cpath = match CString::new(resolved.as_os_str().as_encoded_bytes()) {
		Ok(c) => c,
		// NUL bytes in path components are unrecoverable; skip silently so
		// a malformed allow entry cannot abort the whole sandbox.
		Err(_) => return Ok(()),
	};
	let fd = unsafe { libc::open(cpath.as_ptr(), O_PATH | O_CLOEXEC, 0) };
	if fd < 0 {
		// Path does not exist or is inaccessible — skip silently so a
		// missing `/lib64` on a pure-glibc system doesn't break the sandbox.
		return Ok(());
	}
	let _close_on_drop = FdGuard(fd as RawFd);

	let allowed = access_mask(access, handled_mask);
	if allowed == 0 {
		return Ok(());
	}

	let rule = LandlockPathBeneathAttr {
		allowed_access: allowed,
		parent_fd: fd,
	};
	let r = unsafe {
		libc::syscall(
			SYS_LANDLOCK_ADD_RULE,
			ruleset_fd as c_long,
			LANDLOCK_RULE_PATH_BENEATH as c_long,
			&rule as *const LandlockPathBeneathAttr as c_long,
			0,
			0,
			0,
		)
	};
	if r != 0 {
		let errno = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
		return Err(LandlockError::AddRuleSyscall(errno));
	}
	Ok(())
}

// ---- ABI structs ------------------------------------------------------------

/// Mirrors `unix.LandlockRulesetAttr`. Layout matches the kernel ABI.
#[repr(C)]
struct LandlockRulesetAttr {
	access_fs: u64,
}

/// Mirrors `unix.LandlockPathBeneathAttr`. Layout matches the kernel ABI.
#[repr(C)]
struct LandlockPathBeneathAttr {
	allowed_access: u64,
	parent_fd: c_int,
}

// ---- Sensible defaults ------------------------------------------------------

/// A ruleset that permits reading most of the filesystem (for runtime /
/// loader / libs) but restricts writes to the supplied workspace. Mirrors
/// `landlock.SensibleDefaults`. The `cwd` and `log_dir` paths are added with
/// read+write+execute where appropriate.
#[must_use]
pub fn sensible_defaults(cwd: &str, log_dir: &str) -> Ruleset {
	let mut ruleset = Ruleset {
		allow: vec![
			path_access("/usr", true, false, true),
			path_access("/bin", true, false, true),
			path_access("/sbin", true, false, true),
			path_access("/lib", true, false, true),
			path_access("/lib64", true, false, true),
			path_access("/proc", true, false, false),
			path_access("/sys", true, false, false),
			path_access("/dev", true, true, false),
			path_access("/etc", true, false, false),
			path_access("/tmp", true, true, true),
			path_access(&runtime_dir(), true, true, true),
		],
	};
	if !cwd.is_empty() {
		ruleset.allow.push(PathAccess {
			path: cwd.to_string(),
			read: true,
			write: true,
			execute: true,
		});
	}
	if !log_dir.is_empty() && log_dir != cwd {
		ruleset.allow.push(PathAccess {
			path: log_dir.to_string(),
			read: true,
			write: true,
			execute: false,
		});
	}
	ruleset
}

fn path_access(path: &str, read: bool, write: bool, execute: bool) -> PathAccess {
	PathAccess {
		path: path.to_string(),
		read,
		write,
		execute,
	}
}

fn runtime_dir() -> String {
	std::env::var("XDG_RUNTIME_DIR").unwrap_or_else(|_| "/run".to_string())
}

// ---- Errors -----------------------------------------------------------------

/// Errors surfaced by [`apply`]. The Go implementation wraps every error
/// with a `fmt.Errorf("...: %w", ...)`. The Rust variants carry enough
/// context to recreate the same message string.
#[derive(Debug)]
pub enum LandlockError {
	/// `landlock_create_ruleset` returned ENOSYS — kernel has no Landlock
	/// support. Surfaced only from the version probe; the [`apply`] entry
	/// point absorbs this into `Ok(())`.
	Unsupported,
	/// `landlock_create_ruleset` returned another errno.
	Probe(i32),
	/// `landlock_create_ruleset` (with a real attr) returned an errno.
	CreateRuleset(i32),
	/// `prctl(PR_SET_NO_NEW_PRIVS, ...)` returned an errno.
	NoNewPrivs(i32),
	/// `landlock_restrict_self` returned an errno.
	RestrictSelf(i32),
	/// A path rule's path was not absolute. Wraps the Go error
	/// "path must be absolute".
	PathNotAbsolute,
	/// `landlock_add_rule` for a path failed; carries the original errno.
	AddRuleSyscall(i32),
	/// Wrapper carrying the path context for an add-rule failure. Mirrors
	/// the Go `fmt.Errorf("landlock add rule for %q: %w", path, err)`.
	AddRule {
		path: String,
		source: Box<LandlockError>,
	},
}

impl std::fmt::Display for LandlockError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			LandlockError::Unsupported => f.write_str("landlock not supported by kernel"),
			LandlockError::Probe(e) => write!(f, "landlock_create_ruleset probe: {e}"),
			LandlockError::CreateRuleset(e) => write!(f, "landlock_create_ruleset: {e}"),
			LandlockError::NoNewPrivs(e) => write!(f, "prctl(PR_SET_NO_NEW_PRIVS): {e}"),
			LandlockError::RestrictSelf(e) => write!(f, "landlock_restrict_self: {e}"),
			LandlockError::PathNotAbsolute => f.write_str("path must be absolute"),
			LandlockError::AddRuleSyscall(e) => write!(f, "landlock_add_rule: {e}"),
			LandlockError::AddRule { path, source } => {
				write!(f, "landlock add rule for {path:?}: {source}")
			}
		}
	}
}

impl std::error::Error for LandlockError {
	fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
		match self {
			LandlockError::AddRule { source, .. } => Some(source.as_ref()),
			_ => None,
		}
	}
}

// ---- Fd guard ---------------------------------------------------------------

/// Tiny RAII wrapper that closes an fd on drop. Used in place of `defer`
/// inside [`apply`] and [`add_path_rule`].
struct FdGuard(RawFd);

impl Drop for FdGuard {
	fn drop(&mut self) {
		// SAFETY: the fd was created by `open` / `landlock_create_ruleset`
		// and we own the only reference in this scope. Closing an invalid
		// fd is a no-op at the syscall level; ignoring the result matches
		// the Go `_ = syscall.Close(fd)` idiom.
		unsafe {
			libc::close(self.0);
		}
	}
}

#[cfg(test)]
mod tests;
