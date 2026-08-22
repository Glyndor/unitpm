//! `SO_PEERCRED`-based peer authentication for Unix-socket clients.
//!
//! The peer identity is fetched straight from the kernel — there is no
//! userland trust boundary to thread through, which is the point. The
//! `UNITPM_IPC_ALLOW_UIDS` allowlist lets operators narrow the trust to a
//! specific set of users when running rootless.

use std::env;
use std::io;
use std::os::unix::net::UnixStream;

use crate::ipc::transport::Identity;

/// Errors surfaced by [`validate_identity`].
#[derive(Debug)]
pub enum IdentityError {
	NotUnix,
	PeerCred(io::Error),
	Unauthorized { uid: i32, reason: String },
}

impl std::fmt::Display for IdentityError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			IdentityError::NotUnix => f.write_str("invalid connection type"),
			IdentityError::PeerCred(e) => write!(f, "peercred: {e}"),
			IdentityError::Unauthorized { uid, reason } => {
				write!(f, "unauthorized user {uid}: {reason}")
			}
		}
	}
}

impl std::error::Error for IdentityError {}

/// Fetch the peer's [`Identity`] via `SO_PEERCRED`. The `UNITPM_IPC_ALLOW_UIDS`
/// allowlist and the system-daemon / user-daemon distinction are applied here.
pub fn validate_identity(stream: &UnixStream) -> Result<Identity, IdentityError> {
	let cred = peer_cred(stream).map_err(IdentityError::PeerCred)?;

	let daemon_uid = unsafe { libc::geteuid() } as i32;
	let client_uid = cred.uid as i32;

	if let Ok(allow_str) = env::var("UNITPM_IPC_ALLOW_UIDS") {
		if !allow_str.is_empty() {
			let allowed = allow_str
				.split(',')
				.map(str::trim)
				.filter(|s| !s.is_empty())
				.filter_map(|s| s.parse::<i32>().ok())
				.any(|id| id == client_uid);
			if !allowed {
				return Err(IdentityError::Unauthorized {
					uid: client_uid,
					reason: "not in allowlist".into(),
				});
			}
		}
	}

	let mut is_system_daemon = daemon_uid == 0;
	if !is_system_daemon {
		if let Ok(name) = lookup_user_by_uid(daemon_uid as u32) {
			if name == "unitpm" {
				is_system_daemon = true;
			}
		}
	}

	if !is_system_daemon && client_uid != daemon_uid {
		return Err(IdentityError::Unauthorized {
			uid: client_uid,
			reason: format!("daemon uid: {daemon_uid}"),
		});
	}

	Ok(Identity {
		uid: cred.uid.to_string(),
		gid: cred.gid.to_string(),
		pid: cred.pid as i32,
	})
}

#[repr(C)]
#[derive(Debug, Clone, Copy)]
struct Ucred {
	pid: libc::pid_t,
	uid: libc::uid_t,
	gid: libc::gid_t,
}

/// `SO_PEERCRED` wrapper around [`UnixStream`]. Avoids the unstable
/// `peer_credentials_unix_socket` feature while keeping the call site local.
fn peer_cred(stream: &UnixStream) -> io::Result<Ucred> {
	let fd = stream.as_raw_fd();
	let mut cred: Ucred = unsafe { std::mem::zeroed() };
	let mut len = std::mem::size_of::<Ucred>() as libc::socklen_t;
	let r = unsafe {
		libc::getsockopt(
			fd,
			libc::SOL_SOCKET,
			libc::SO_PEERCRED,
			&mut cred as *mut _ as *mut libc::c_void,
			&mut len,
		)
	};
	if r != 0 {
		return Err(io::Error::last_os_error());
	}
	if (len as usize) < std::mem::size_of::<Ucred>() {
		return Err(io::Error::other("SO_PEERCRED returned truncated ucred"));
	}
	Ok(cred)
}

use std::os::fd::AsRawFd;

/// Best-effort lookup of the user name for `uid`. Returns `Ok(None)` when the
/// entry is missing or the lookup errors.
fn lookup_user_by_uid(uid: u32) -> io::Result<String> {
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
			return Err(io::Error::last_os_error());
		}
		let pw = &*result;
		let name = std::ffi::CStr::from_ptr(pw.pw_name);
		Ok(name.to_string_lossy().into_owned())
	}
}
