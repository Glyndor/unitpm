//! Lifecycle verbs whose request is `{id}` and response is `{status, id}`.
//!
//! Restart / reload / reset are templated on top of [`register_id_verb`].
//! They share an audit path and differ only in the manager method they call
//! and the past-tense string they return.

use crate::daemon::audit::Logger;
use crate::daemon::handlers::audit::audit_event;
use crate::daemon::handlers::service::SharedManager;
use crate::ipc::protocol::RawMessage;
use crate::ipc::transport::RequestContext;
use crate::jsonx;

/// Wire a verb whose request is `{id}` and response is `{status, id}`.
///
/// `action` runs against the [`SharedManager`] and returns the manager's
/// error verbatim. `past_tense` is the value used for `status` in the
/// response payload (`"restarted"`, `"reloaded"`, `"reset"`).
pub fn register_id_verb(
	mgr: SharedManager,
	auditor: std::sync::Arc<Logger>,
	verb: &'static str,
	past_tense: &'static str,
	action: fn(&SharedManager, &str) -> Result<(), String>,
) -> impl Fn(RequestContext, RawMessage) -> Result<RawMessage, String> + Send + Sync + 'static {
	move |ctx, params| {
		let args: IdArgs =
			jsonx::unmarshal(params.as_bytes()).map_err(|e| format!("ERR_BAD_REQUEST: {e}"))?;

		let id = match mgr
			.lock()
			.unwrap_or_else(|e| e.into_inner())
			.resolve_id(&args.id)
		{
			Ok(id) => id,
			Err(e) => {
				audit_event(
					&auditor,
					&ctx,
					verb,
					&args.id,
					"",
					"",
					false,
					Some(&e.to_string()),
				);
				return Err(e.to_string());
			}
		};

		match action(&mgr, &id) {
			Ok(()) => {
				let (name, ns) = crate::daemon::handlers::audit::process_meta(&mgr, &id);
				audit_event(&auditor, &ctx, verb, &id, &name, &ns, true, None);
				let resp = serde_json::json!({"status": past_tense, "id": id});
				jsonx::marshal(&resp)
					.map(|b| RawMessage::from_bytes(&b))
					.map_err(|e| e.to_string())
			}
			Err(e) => {
				audit_event(&auditor, &ctx, verb, &id, "", "", false, Some(&e));
				Err(e)
			}
		}
	}
}

#[derive(Debug, serde::Deserialize)]
struct IdArgs {
	id: String,
}

pub fn restart(mgr: &SharedManager, id: &str) -> Result<(), String> {
	mgr.lock()
		.unwrap_or_else(|e| e.into_inner())
		.restart(id)
		.map_err(|e| e.to_string())
}

pub fn reload(mgr: &SharedManager, id: &str) -> Result<(), String> {
	mgr.lock()
		.unwrap_or_else(|e| e.into_inner())
		.reload(id)
		.map_err(|e| e.to_string())
}

pub fn reset(mgr: &SharedManager, id: &str) -> Result<(), String> {
	mgr.lock()
		.unwrap_or_else(|e| e.into_inner())
		.reset(id)
		.map_err(|e| e.to_string())
}
