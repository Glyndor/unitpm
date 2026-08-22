//! System-mode detection.
//!
//! Linux-only because the `unitpm` system user is provisioned by the Debian
//! package and is not meaningful on other platforms.

/// Dedicated unprivileged UID the Debian package provisions for the daemon.
pub const SYSTEM_USER: &str = "unitpm";

/// Whether the daemon is the system-mode service — running as root, or as the
/// dedicated system user installed by the Debian package. Both share the
/// same trust posture: requests come from the privileged group via
/// `/run/unitpmd/unitpm.sock` and writes target the system layout under
/// `/var/{lib,log}/unitpm`.
#[must_use]
pub fn is_system_mode() -> bool {
	if super::is_root() {
		return true;
	}
	current_username() == SYSTEM_USER
}

fn current_username() -> String {
	#[cfg(unix)]
	unsafe {
		let login = libc::getlogin();
		if login.is_null() {
			String::new()
		} else {
			std::ffi::CStr::from_ptr(login)
				.to_string_lossy()
				.into_owned()
		}
	}
	#[cfg(not(unix))]
	{
		String::new()
	}
}
