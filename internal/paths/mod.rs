//! XDG-aware filesystem path resolution.
//!
//! Owns the system-mode constants (`LOG_ROOT`, `RUN_DIR`, `CREDS_DIR`,
//! `DATA_DIR`) and the helpers that resolve log directories for both
//! system-mode and user-mode deployments. System-mode privileges are detected
//! via [`is_root`] (universal) plus the Linux-only [`is_system_mode`] (which
//! also matches the dedicated system user installed by the Debian package).

mod system_mode;

#[cfg(target_os = "linux")]
pub use system_mode::{is_system_mode, SYSTEM_USER};

use std::path::{Component, Path, PathBuf};
use std::sync::atomic::{AtomicI32, Ordering};

/// System-mode directory where the daemon writes per-process logs.
pub const LOG_ROOT: &str = "/var/log/glyndor/unitpm";
/// System-mode runtime directory that holds the IPC socket.
pub const RUN_DIR: &str = "/run/unitpmd";
/// Where systemd `LoadCredential=` staging files are written for
/// `--isolation dynamic`, one subdirectory per process ID.
pub const CREDS_DIR: &str = "/var/lib/glyndor/unitpm/creds";
/// Persistent state root for the system user.
pub const DATA_DIR: &str = "/var/lib/glyndor/unitpm";

const MAX_DIR_LEN: usize = 4096;

static EUID_OVERRIDE: AtomicI32 = AtomicI32::new(-1);

fn real_euid() -> u32 {
	#[cfg(unix)]
	unsafe {
		libc::geteuid() as u32
	}
	#[cfg(not(unix))]
	{
		0
	}
}

/// Read the current effective UID. Tests may override via [`set_euid_for_tests`].
pub(crate) fn current_euid() -> u32 {
	let v = EUID_OVERRIDE.load(Ordering::Relaxed);
	if v >= 0 {
		v as u32
	} else {
		real_euid()
	}
}

#[cfg(test)]
pub(crate) fn set_euid_for_tests(v: u32) {
	EUID_OVERRIDE.store(v as i32, Ordering::Relaxed);
}

#[cfg(test)]
pub(crate) fn clear_euid_for_tests() {
	EUID_OVERRIDE.store(-1, Ordering::Relaxed);
}

/// Whether the current process is running as root (euid 0).
#[must_use]
pub fn is_root() -> bool {
	current_euid() == 0
}

/// Resolve the root log directory. If `configured_dir` is empty, the default
/// for the current mode is returned; otherwise the supplied directory is
/// validated against the current mode's allowlist.
pub fn get_log_dir(configured_dir: &str) -> Result<PathBuf, PathError> {
	if configured_dir.is_empty() {
		return resolve_default_dir();
	}
	resolve_configured_dir(configured_dir)
}

fn resolve_configured_dir(dir: &str) -> Result<PathBuf, PathError> {
	if dir.len() > MAX_DIR_LEN {
		return Err(PathError::TooLong);
	}
	if has_parent_dir_traversal(dir) {
		return Err(PathError::Invalid);
	}

	#[cfg(target_os = "linux")]
	if is_system_mode() {
		return resolve_root_log_dir(Path::new(dir));
	}

	Ok(PathBuf::from(dir))
}

/// `true` when `dir`, after lexical normalisation, equals `..` or starts with
/// `../`. Matches the Go `filepath.Clean(...) == ".."` /
/// `strings.HasPrefix(clean, "../")` check.
fn has_parent_dir_traversal(dir: &str) -> bool {
	let mut components = Path::new(dir).components();
	matches!(components.next(), Some(Component::ParentDir))
}

#[cfg(target_os = "linux")]
fn resolve_root_log_dir(candidate: &Path) -> Result<PathBuf, PathError> {
	if !candidate.is_absolute() {
		return Err(PathError::NotAbsolute);
	}

	let mut allowed_roots: Vec<PathBuf> = vec![PathBuf::from(LOG_ROOT)];
	if let Ok(state) = std::env::var("XDG_STATE_HOME") {
		if !state.is_empty() {
			allowed_roots.push(PathBuf::from(state).join("unitpm/logs"));
		}
	}

	let mut resolved_roots: Vec<PathBuf> = Vec::with_capacity(allowed_roots.len());
	for root in allowed_roots {
		let abs = if root.is_absolute() {
			root.clone()
		} else {
			continue;
		};
		let resolved = std::fs::canonicalize(&abs).unwrap_or(abs);
		resolved_roots.push(resolved);
	}

	for root in &resolved_roots {
		if !within_root(root, candidate) {
			continue;
		}
		if match_resolved_root(root, candidate) {
			return Ok(candidate.to_path_buf());
		}
	}
	Err(PathError::OutsideAllowedRoots)
}

/// Reports whether `candidate` resolves safely inside `root`. When `candidate`
/// exists it is canonicalised and compared; otherwise each path component is
/// scanned for symlinks that would escape `root`. The scan closes the TOCTOU
/// race where a symlink is planted between the check and the first write.
fn match_resolved_root(root: &Path, candidate: &Path) -> bool {
	if let Ok(resolved) = std::fs::canonicalize(candidate) {
		return within_root(root, &resolved);
	}
	within_root(root, candidate) && !path_contains_unsafe_symlink(root, candidate)
}

fn resolve_default_dir() -> Result<PathBuf, PathError> {
	#[cfg(target_os = "linux")]
	if is_system_mode() {
		return Ok(PathBuf::from(LOG_ROOT));
	}
	if let Ok(state) = std::env::var("XDG_STATE_HOME") {
		if !state.is_empty() {
			return Ok(PathBuf::from(state).join("unitpm/logs"));
		}
	}
	let home = std::env::var("HOME").map_err(|_| PathError::NoHome)?;
	Ok(PathBuf::from(home).join(".local/state/unitpm/logs"))
}

/// Reports whether `path` is inside `root` after lexically normalising `..`.
#[must_use]
pub fn within_root(root: &Path, path: &Path) -> bool {
	let rel = match path.strip_prefix(root) {
		Ok(r) => r,
		Err(_) => return false,
	};
	for comp in rel.components() {
		if matches!(comp, Component::ParentDir) {
			return false;
		}
	}
	true
}

fn path_contains_unsafe_symlink(root: &Path, path: &Path) -> bool {
	let rel = match path.strip_prefix(root) {
		Ok(r) => r,
		Err(_) => return true,
	};
	let mut current = root.to_path_buf();
	for part in rel.components() {
		let segment = match part {
			Component::Normal(s) => s,
			_ => continue,
		};
		current.push(segment);
		let meta = match std::fs::symlink_metadata(&current) {
			Ok(m) => m,
			Err(e) => return e.kind() != std::io::ErrorKind::NotFound,
		};
		if meta.file_type().is_symlink() {
			let resolved = match std::fs::canonicalize(&current) {
				Ok(r) => r,
				Err(_) => return true,
			};
			if !within_root(root, &resolved) {
				return true;
			}
		}
	}
	false
}

/// Resolve absolute paths for stdout and stderr logs of a given spec.
pub fn resolve_log_paths(
	spec_id: &str,
	logs_dir: &str,
	stdout: &str,
	stderr: &str,
) -> Result<(PathBuf, PathBuf), PathError> {
	let log_dir = get_log_dir(logs_dir)?;
	let app_log_dir = log_dir.join(spec_id);

	let stdout_path = if stdout.is_empty() {
		app_log_dir.join("stdout.log")
	} else if Path::new(stdout).is_absolute() {
		PathBuf::from(stdout)
	} else {
		app_log_dir.join(stdout)
	};

	let stderr_path = if stderr.is_empty() {
		app_log_dir.join("stderr.log")
	} else if Path::new(stderr).is_absolute() {
		PathBuf::from(stderr)
	} else {
		app_log_dir.join(stderr)
	};

	Ok((stdout_path, stderr_path))
}

/// Errors surfaced by the path helpers.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PathError {
	TooLong,
	Invalid,
	#[cfg(target_os = "linux")]
	NotAbsolute,
	#[cfg(target_os = "linux")]
	OutsideAllowedRoots,
	NoHome,
}

impl std::fmt::Display for PathError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			PathError::TooLong => f.write_str("log dir too long"),
			PathError::Invalid => f.write_str("invalid log dir"),
			#[cfg(target_os = "linux")]
			PathError::NotAbsolute => f.write_str("invalid log dir: must be absolute in system mode"),
			#[cfg(target_os = "linux")]
			PathError::OutsideAllowedRoots => f.write_str("invalid log dir: outside allowed roots"),
			PathError::NoHome => f.write_str("failed to get user home"),
		}
	}
}

impl std::error::Error for PathError {}

#[cfg(all(test, target_os = "linux"))]
mod linux_tests;
