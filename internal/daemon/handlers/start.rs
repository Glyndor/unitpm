//! The `start` verb — the daemon's front door.
//!
//! Mirrors `internal/daemon/handlers/start.go`. Parses the
//! [`StartRequest`](crate::ipc::protocol::StartRequest) envelope, hands
//! the spec to [`start_process`](super::service::start_process), and
//! packages the response as a [`StartResponseData`].
//!
//! Peer identity comes from the [`RequestContext`] the dispatcher attaches
//! to every request — `SO_PEERCRED` populated it before the handler ran.

use crate::daemon::handlers::service::{start_process, SharedManager};
use crate::ipc::protocol::{RawMessage, StartRequest, StartResponseData};
use crate::ipc::transport::RequestContext;
use crate::jsonx;

/// Build the `start` handler. `privileged` is the daemon's own mode —
/// `true` for the system instance (root or `unitpm`), `false` for a
/// user-mode daemon. The `policy::authorize_start` check enforces the
/// security boundary.
pub fn start_handler(
	mgr: SharedManager,
	privileged: bool,
) -> impl Fn(RequestContext, RawMessage) -> Result<RawMessage, String> + Send + Sync + 'static {
	move |ctx, params| {
		let req: StartRequest =
			jsonx::unmarshal(params.as_bytes()).map_err(|e| format!("ERR_BAD_REQUEST: {e}"))?;

		if req.spec.id.is_empty() {
			return Err("ERR_BAD_REQUEST: spec ID is required".into());
		}

		let info = start_process(&mgr, req.spec, &ctx.identity, privileged)?;

		let data = StartResponseData {
			id: info.id.clone(),
			proc_id: Some(info.id),
			pid: Some(info.pid as i32),
			status: Some(info.state.as_str().to_string()),
			message: None,
			created_at: None,
		};

		jsonx::marshal(&data)
			.map(|b| RawMessage::from_bytes(&b))
			.map_err(|e| e.to_string())
	}
}
