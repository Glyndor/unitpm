//! Resolve the IPC socket path.
//!
//! Order of precedence (mirrors the Go daemon):
//!
//! 1. `UNITPM_SOCKET` env override — must be absolute and the parent
//!    directory must not be world-writable.
//! 2. System socket at `/run/unitpmd/unitpm.sock` when running as the system
//!    service (root or `unitpm` user).
//! 3. System socket for members of the `unitpmadm` group when one exists.
//! 4. Per-user socket under `$XDG_RUNTIME_DIR/unitpm-<uid>/unitpm.sock`. The
//!    directory is created `0700` if missing. We refuse to fall back to
//!    `/tmp` because that would let any local user pre-create a symlink at
//!    `/tmp/unitpm-<victimUid>` and hijack the socket on the next run.

use std::env;
use std::io;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

const SYSTEM_SOCKET: &str = "/run/unitpmd/unitpm.sock";

/// Errors surfaced by [`get_socket_path`].
#[derive(Debug)]
pub enum SocketPathError {
	RelativeEnv(String),
	WorldWritableDir(String),
	NoXdgRuntime,
	HomeResolution,
	Mkdir(String, io::Error),
	Chmod(String, io::Error),
}

impl std::fmt::Display for SocketPathError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			SocketPathError::RelativeEnv(p) => {
				write!(f, "UNITPM_SOCKET must be an absolute path, got: {p}")
			}
			SocketPathError::WorldWritableDir(d) => write!(
				f,
				"UNITPM_SOCKET parent directory {d} is world-writable: insecure"
			),
			SocketPathError::NoXdgRuntime => f.write_str(
				"XDG_RUNTIME_DIR is not set; run under a login session \
				 (ssh, systemd-user) or export UNITPM_SOCKET to an absolute \
				 path in a private directory",
			),
			SocketPathError::HomeResolution => f.write_str("failed to resolve current user"),
			SocketPathError::Mkdir(p, e) => write!(f, "failed to create socket directory {p}: {e}"),
			SocketPathError::Chmod(p, e) => {
				write!(f, "failed to set socket directory permissions {p}: {e}")
			}
		}
	}
}

impl std::error::Error for SocketPathError {}

/// Return the canonical path to the IPC socket. The result is suitable for
/// `UnixStream::connect` or `UnixListener::bind`.
pub fn get_socket_path() -> Result<String, SocketPathError> {
	if let Ok(env_path) = env::var("UNITPM_SOCKET") {
		if env_path.is_empty() {
			// fall through to default resolution
		} else {
			return validate_env_override(&env_path);
		}
	}

	let uid = unsafe { libc::geteuid() };
	let username = lookup_user_by_uid(uid as u32).unwrap_or_default();

	if uid == 0 || username == "unitpm" {
		return Ok(SYSTEM_SOCKET.to_string());
	}

	// Admin exception: prefer the system socket when the user is in the
	// `unitpmadm` group, unless they already have a personal socket.
	if Path::new(SYSTEM_SOCKET).exists() {
		let user_socket = user_socket_path_for(uid)?;
		if !user_socket.exists() {
			if let Some(gids) = group_ids_for(uid) {
				if let Some(adm_gid) = lookup_group_gid("unitpmadm") {
					if gids.contains(&adm_gid) {
						return Ok(SYSTEM_SOCKET.to_string());
					}
				}
			}
		}
	}

	let user_socket = user_socket_path_for(uid)?;
	if let Some(dir) = user_socket.parent() {
		std::fs::create_dir_all(dir)
			.map_err(|e| SocketPathError::Mkdir(dir.display().to_string(), e))?;
		std::fs::set_permissions(dir, std::fs::Permissions::from_mode(0o700))
			.map_err(|e| SocketPathError::Chmod(dir.display().to_string(), e))?;
	}

	Ok(user_socket.display().to_string())
}

fn validate_env_override(path: &str) -> Result<String, SocketPathError> {
	let p = Path::new(path);
	if !p.is_absolute() {
		return Err(SocketPathError::RelativeEnv(path.to_string()));
	}
	if let Some(dir) = p.parent() {
		if dir.exists() {
			let info = std::fs::metadata(dir).map_err(|_| SocketPathError::HomeResolution)?;
			if info.permissions().mode() & 0o002 != 0 {
				return Err(SocketPathError::WorldWritableDir(dir.display().to_string()));
			}
		}
	}
	Ok(path.to_string())
}

fn user_socket_path_for(uid: u32) -> Result<PathBuf, SocketPathError> {
	let base = env::var("XDG_RUNTIME_DIR")
		.ok()
		.filter(|s| !s.is_empty())
		.ok_or(SocketPathError::NoXdgRuntime)?;
	Ok(Path::new(&base)
		.join(format!("unitpm-{uid}"))
		.join("unitpm.sock"))
}

fn lookup_user_by_uid(uid: u32) -> Option<String> {
	unsafe {
		let mut pwd: libc::passwd = std::mem::zeroed();
		let mut buf = vec![0u8; 1024];
		let mut result = std::ptr::null_mut();
		let r = libc::getpwuid_r(
			uid,
			&mut pwd,
			buf.as_mut_ptr() as *mut libc::c_char,
			buf.len(),
			&mut result,
		);
		if r != 0 || result.is_null() {
			return None;
		}
		let pw = &*result;
		let name = std::ffi::CStr::from_ptr(pw.pw_name);
		Some(name.to_string_lossy().into_owned())
	}
}

fn lookup_group_gid(name: &str) -> Option<u32> {
	unsafe {
		let mut grp: libc::group = std::mem::zeroed();
		let mut buf = vec![0u8; 1024];
		let mut result = std::ptr::null_mut();
		let r = libc::getgrnam_r(
			name.as_ptr() as *const libc::c_char,
			&mut grp,
			buf.as_mut_ptr() as *mut libc::c_char,
			buf.len(),
			&mut result,
		);
		if r != 0 || result.is_null() {
			return None;
		}
		let gr = &*result;
		Some(gr.gr_gid)
	}
}

fn group_ids_for(uid: u32) -> Option<Vec<u32>> {
	unsafe {
		let mut pwd: libc::passwd = std::mem::zeroed();
		let mut buf = vec![0u8; 1024];
		let mut result = std::ptr::null_mut();
		let r = libc::getpwuid_r(
			uid,
			&mut pwd,
			buf.as_mut_ptr() as *mut libc::c_char,
			buf.len(),
			&mut result,
		);
		if r != 0 || result.is_null() {
			return None;
		}
		let pw = &*result;
		let mut ngroups: libc::c_int = 64;
		let mut groups: Vec<libc::gid_t> = vec![0; ngroups as usize];
		let r = libc::getgrouplist(pw.pw_name, pw.pw_gid, groups.as_mut_ptr(), &mut ngroups);
		if r < 0 {
			return None;
		}
		groups.truncate(ngroups as usize);
		Some(groups.into_iter().collect())
	}
}

#[cfg(test)]
mod tests;
