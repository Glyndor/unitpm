//! Read-only verbs that do not need the [`SharedManager`] lock to respond.
//!
//! - `ping`, `version` — no params.
//! - `list`, `show`, `proctree` — read-only views over the manager.
//!
//! Each handler returns its response as a [`RawMessage`] (already-encoded
//! JSON). The dispatcher in [`server_dispatch`](crate::ipc::transport::server_dispatch)
//! wraps it into the wire envelope.

use crate::daemon::handlers::service::SharedManager;
use crate::ipc::protocol::RawMessage;
use crate::ipc::transport::RequestContext;
use crate::jsonx;
use crate::spec;
use crate::version;

/// Marshal `value` into a [`RawMessage`] (already-encoded JSON bytes).
/// Handlers return `RawMessage`; the dispatcher wraps it in the wire envelope.
pub(crate) fn marshal<T: serde::Serialize>(value: &T) -> Result<RawMessage, String> {
	jsonx::marshal(value)
		.map(|b| RawMessage::from_bytes(&b))
		.map_err(|e| e.to_string())
}

#[derive(Debug, serde::Deserialize)]
pub(crate) struct IdArgs {
	pub id: String,
}

/// `ping` — no params, no audit. Returns `{response: "pong"}`.
pub fn ping(_ctx: RequestContext, _params: RawMessage) -> Result<RawMessage, String> {
	let resp = serde_json::json!({"response": "pong"});
	marshal(&resp)
}

/// `version` — no params, no audit. Returns the build [`Info`](version::Info).
pub fn version_handler(_ctx: RequestContext, _params: RawMessage) -> Result<RawMessage, String> {
	jsonx::marshal(&version::get())
		.map(|b| RawMessage::from_bytes(&b))
		.map_err(|e| e.to_string())
}

/// `list` — every process in the manager, no params.
pub fn list_handler(
	mgr: SharedManager,
	_ctx: RequestContext,
	_params: RawMessage,
) -> Result<RawMessage, String> {
	let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
	marshal(&guard.list())
}

/// `show` — one process's info+spec. Falls back to the on-disk spec when
/// the process is no longer live.
pub fn show_handler(
	mgr: SharedManager,
	_ctx: RequestContext,
	params: RawMessage,
) -> Result<RawMessage, String> {
	let args: IdArgs =
		jsonx::unmarshal(params.as_bytes()).map_err(|e| format!("ERR_BAD_REQUEST: {e}"))?;

	let id = mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.resolve_id(&args.id)
		.map_err(|e| e.to_string())?;

	let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
	if let Some(proc) = guard.get(&id) {
		let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
		let resp = serde_json::json!({"info": p.info(), "spec": p.spec_copy()});
		return marshal(&resp);
	}
	drop(guard);

	let s = spec::load_spec_protocol(&id).map_err(|_| format!("process not found: {}", args.id))?;
	let resp = serde_json::json!({"spec": s});
	marshal(&resp)
}

/// `proctree` — `/proc` snapshot of the process and its descendants.
pub fn proctree_handler(
	mgr: SharedManager,
	_ctx: RequestContext,
	params: RawMessage,
) -> Result<RawMessage, String> {
	let args: IdArgs =
		jsonx::unmarshal(params.as_bytes()).map_err(|e| format!("ERR_BAD_REQUEST: {e}"))?;

	let id = mgr
		.lock()
		.unwrap_or_else(|e| e.into_inner())
		.resolve_id(&args.id)
		.map_err(|e| e.to_string())?;

	let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
	let proc = guard
		.get(&id)
		.ok_or_else(|| format!("process {:?} not found", args.id))?;
	let p = proc.lock().unwrap_or_else(|e| e.into_inner());
	let tree = p.tree().unwrap_or_default();
	marshal(&tree)
}
