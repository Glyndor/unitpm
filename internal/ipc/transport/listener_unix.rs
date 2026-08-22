//! Unix-socket listener setup.
//!
//! Tightens the umask so the freshly created socket does not leak across
//! groups, and enforces ownership/permission rules on the system-mode
//! directory before binding. Returns a [`UnixListener`] ready to accept.

use std::io;
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::UnixListener;
use std::path::Path;

/// Errors surfaced by [`listen`].
#[derive(Debug)]
pub enum ListenError {
	Io(io::Error),
	WorldWritableDir(String),
	BadDirOwnership {
		dir: String,
		owner: u32,
		expected: u32,
	},
}

impl std::fmt::Display for ListenError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			ListenError::Io(e) => write!(f, "io error: {e}"),
			ListenError::WorldWritableDir(d) => {
				write!(f, "socket directory {d} is world-writable: insecure")
			}
			ListenError::BadDirOwnership {
				dir,
				owner,
				expected,
			} => write!(
				f,
				"socket directory {dir} is owned by uid {owner}, expected root (0) or daemon user ({expected})"
			),
		}
	}
}

impl std::error::Error for ListenError {}

const SYSTEM_SOCKET_PATH: &str = "/run/unitpmd/unitpm.sock";

/// Bind a Unix socket at `path` with safe defaults. Returns the listener
/// after tightening its permissions.
pub fn listen<P: AsRef<Path>>(path: P) -> Result<UnixListener, ListenError> {
	let path = path.as_ref();

	// Remove any stale socket from a previous daemon instance.
	if path.exists() {
		std::fs::remove_file(path).map_err(ListenError::Io)?;
	}

	let is_system = path == Path::new(SYSTEM_SOCKET_PATH);

	// Set umask so the freshly-created socket file is private. The previous
	// umask is restored on the way out — Rust does not have a deferred
	// umask reset, so we lean on RAII: a no-op Drop guard reverts it when
	// this function returns.
	let old_mask = set_umask(0o077);

	let result = listen_inner(path, is_system);

	// Always restore, even on the error path. The Drop guard `UmaskGuard`
	// would be cleaner but a plain restore keeps this file simple.
	set_umask(old_mask);

	result
}

fn listen_inner(path: &Path, is_system: bool) -> Result<UnixListener, ListenError> {
	if is_system {
		if let Some(dir) = path.parent() {
			std::fs::create_dir_all(dir).map_err(ListenError::Io)?;
			std::fs::set_permissions(dir, std::fs::Permissions::from_mode(0o755))
				.map_err(ListenError::Io)?;
			let info = std::fs::metadata(dir).map_err(ListenError::Io)?;
			if info.permissions().mode() & 0o002 != 0 {
				return Err(ListenError::WorldWritableDir(dir.display().to_string()));
			}
			verify_dir_owner(dir)?;
		}
	}

	let listener = UnixListener::bind(path).map_err(ListenError::Io)?;

	// The cap is advisory — UnixListener has no built-in back-pressure, but
	// a higher-level semaphore on accept ensures we never exceed it.
	listener.set_nonblocking(false).map_err(ListenError::Io)?;

	if is_system {
		// Make the socket readable by the admin group, if present.
		if let Some(group) = lookup_group("unitpmadm") {
			let _ = std::os::unix::fs::chown(path, None, Some(group));
		}
		std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o660))
			.map_err(ListenError::Io)?;
	} else {
		std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600))
			.map_err(ListenError::Io)?;
	}

	Ok(listener)
}

fn verify_dir_owner(dir: &Path) -> Result<(), ListenError> {
	let info = std::fs::metadata(dir).map_err(ListenError::Io)?;
	use std::os::unix::fs::MetadataExt;
	let owner = info.uid();
	let expected = unsafe { libc::geteuid() } as u32;
	if owner != 0 && owner != expected {
		return Err(ListenError::BadDirOwnership {
			dir: dir.display().to_string(),
			owner,
			expected,
		});
	}
	Ok(())
}

fn lookup_group(name: &str) -> Option<u32> {
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

/// `umask` is process-global, so callers wrap this. Returns the previous
/// mask so the caller can restore it on the way out.
fn set_umask(mask: u16) -> u16 {
	let m: libc::mode_t = mask.into();
	unsafe { libc::umask(m) as u16 }
}

/// Re-export [`MaxConnections`] for convenience.
#[allow(unused_imports)]
pub use crate::ipc::transport::limits::MaxConnections;
