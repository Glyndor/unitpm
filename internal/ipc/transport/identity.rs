//! Authenticated peer identity and the context key that carries it through
//! the dispatcher.

/// Authenticated identity of an IPC client. Populated by [`crate::ipc::transport::validate_identity`]
/// after `SO_PEERCRED` returns the peer's UID, GID, and PID.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct Identity {
	/// User ID as a decimal string.
	pub uid: String,
	/// Group ID as a decimal string.
	pub gid: String,
	/// Process ID of the peer.
	pub pid: i32,
}

/// The context key the dispatcher uses to look up the [`Identity`] attached
/// to each connection. Re-exported so handlers can `ctx.get(ContextKeyIdentity)`
/// without depending on the internal `context_key` newtype.
#[allow(non_upper_case_globals)]
pub const ContextKeyIdentity: &str = "identity";
