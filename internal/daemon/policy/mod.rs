//! Authorization policy for daemon start requests.
//!
//! Mirrors `internal/daemon/policy/policy.go`. The Go test asserts that the
//! returned error message *starts with* the named code (`ERR_UNSUPPORTED` or
//! `ERR_BAD_REQUEST`); the [`PolicyError`] enum below preserves that prefix in
//! its `Display` implementation so the same assertion holds on the Rust side.

use crate::ipc::protocol::AppSpec;
use crate::ipc::transport::Identity;

/// Wire-level error codes the policy can surface. Mirrors the `ERR_*`
/// constants the Go implementation prefixes to its messages.
const CODE_UNSUPPORTED: &str = "ERR_UNSUPPORTED";
const CODE_BAD_REQUEST: &str = "ERR_BAD_REQUEST";

/// Errors returned by [`authorize_start`]. The [`Display`](std::fmt::Display)
/// impl begins with the wire code, so the Go-side assertion that the message
/// starts with the expected code translates directly to a `format!` prefix
/// check on the Rust side.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PolicyError {
	/// `shell: true` was requested against a privileged (system) daemon.
	ShellNotAllowed,
	/// `run_as=dynamic` was requested against a non-privileged (user) daemon.
	DynamicRequiresSystem,
	/// `run_as` requested a mode that is reserved for a later phase
	/// (`app_user`, `explicit_user`).
	ReservedMode(String),
	/// `run_as` carried an unrecognised mode string.
	InvalidMode,
}

impl std::fmt::Display for PolicyError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			PolicyError::ShellNotAllowed => {
				write!(
					f,
					"{CODE_UNSUPPORTED}: shell execution not allowed in system daemon"
				)
			}
			PolicyError::DynamicRequiresSystem => write!(
				f,
				"{CODE_UNSUPPORTED}: run_as=dynamic requires system daemon"
			),
			PolicyError::ReservedMode(m) => write!(
				f,
				"{CODE_UNSUPPORTED}: run_as={m} is not implemented yet; use 'dynamic' or 'sandbox'"
			),
			PolicyError::InvalidMode => write!(f, "{CODE_BAD_REQUEST}: invalid run_as mode"),
		}
	}
}

impl std::error::Error for PolicyError {}

/// Authorize a `start` request. Returns `Ok(())` when the request is allowed.
///
/// Mirrors `policy.AuthorizeStart`. The Go signature takes `*Identity`; the
/// field is unused by the policy and we keep it for caller symmetry. The
/// `daemon_privileged` flag is what the Go code calls `daemonPrivileged` and
/// selects between user-mode (false) and system-mode (true) operation.
pub fn authorize_start(
	spec: &AppSpec,
	_identity: &Identity,
	daemon_privileged: bool,
) -> Result<(), PolicyError> {
	if spec.exec.shell && daemon_privileged {
		return Err(PolicyError::ShellNotAllowed);
	}

	let run_as = match spec.run_as.as_ref() {
		Some(r) => r,
		None => return Ok(()),
	};

	match run_as.mode.as_str() {
		"self" => Ok(()),
		"dynamic" => {
			if !daemon_privileged {
				Err(PolicyError::DynamicRequiresSystem)
			} else {
				Ok(())
			}
		}
		"sandbox" => {
			// Unprivileged sandbox: user namespaces + landlock + rlimit.
			// Works in both user and system mode without sudo.
			Ok(())
		}
		"app_user" | "explicit_user" => Err(PolicyError::ReservedMode(run_as.mode.clone())),
		_ => Err(PolicyError::InvalidMode),
	}
}

#[cfg(test)]
mod tests;
