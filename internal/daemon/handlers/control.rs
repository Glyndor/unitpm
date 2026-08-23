//! Destructive verbs that mutate the [`SharedManager`].
//!
//! - `stop` — graceful stop + record `was_running` for the response payload.
//! - `delete` — stops the process, removes it from the registry, optionally
//!   purges its log directory and its `/var/lib/glyndor/unitpm/creds/<id>`
//!   staging area.
//! - `scale` — multi-process. Audit records `base_name` as the target.
//!
//! `flush` lives in its sibling module because its path-containment logic
//! is the longest of the four and pushes the file past the 500-line gate.
//!
//! Every exit path emits an audit record. Removing `audit_event(...)` from
//! any of these is what the destructive-handler test in
//! `tests/register_tests.rs` catches.

use std::path::Path;

use crate::daemon::audit::Logger;
use crate::daemon::handlers::audit::{audit_event, process_meta, was_running};
use crate::daemon::handlers::query::{marshal, IdArgs};
use crate::daemon::handlers::service::SharedManager;
use crate::ipc::protocol::RawMessage;
use crate::ipc::transport::RequestContext;
use crate::jsonx;
use crate::paths;
use crate::spec;

#[derive(Debug, serde::Deserialize)]
struct DeleteArgs {
	id: String,
	#[serde(default)]
	purge: bool,
}

#[derive(Debug, serde::Deserialize)]
struct ScaleArgs {
	#[serde(default)]
	name: String,
	#[serde(default)]
	namespace: String,
	target: i32,
}

/// `stop` — graceful stop + record `was_running` for the response payload.
/// Audits success and failure on every exit path.
pub fn stop_handler(
	mgr: SharedManager,
	auditor: std::sync::Arc<Logger>,
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
					"stop",
					&args.id,
					"",
					"",
					false,
					Some(&e.to_string()),
				);
				return Err(e.to_string());
			}
		};

		let (name, ns) = process_meta(&mgr, &id);

		let was = {
			let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
			guard
				.get(&id)
				.map(|proc| {
					let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
					was_running(p.info().state)
				})
				.unwrap_or(false)
		};

		let stop_result = mgr
			.lock()
			.unwrap_or_else(|e| e.into_inner())
			.stop(&id)
			.map_err(|e| e.to_string());

		match stop_result {
			Ok(()) => {
				audit_event(&auditor, &ctx, "stop", &id, &name, &ns, true, None);
				let resp = serde_json::json!({
					"status": "stopped",
					"id": id,
					"was_running": was,
				});
				marshal(&resp)
			}
			Err(e) => {
				audit_event(&auditor, &ctx, "stop", &id, &name, &ns, false, Some(&e));
				Err(e)
			}
		}
	}
}

/// `delete` — stops the process, removes it from the registry, optionally
/// purges its log directory and its `/var/lib/glyndor/unitpm/creds/<id>`
/// staging area. Audits on every exit path.
pub fn delete_handler(
	mgr: SharedManager,
	auditor: std::sync::Arc<Logger>,
) -> impl Fn(RequestContext, RawMessage) -> Result<RawMessage, String> + Send + Sync + 'static {
	move |ctx, params| {
		let args: DeleteArgs =
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
					"delete",
					&args.id,
					"",
					"",
					false,
					Some(&e.to_string()),
				);
				return Err(e.to_string());
			}
		};

		// Snapshot name + ns BEFORE deletion so the audit line has useful
		// metadata even after `process_meta` would return empty.
		let (del_name, del_ns) = process_meta(&mgr, &id);

		let app_log_dir = if args.purge {
			let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
			guard.get(&id).and_then(|proc| {
				let p = proc.lock().unwrap_or_else(|e| e.into_inner());
				let configured = p.spec.logs.as_ref().and_then(|l| l.dir.clone());
				drop(p);
				drop(guard);
				configured
			})
		} else {
			None
		};

		let delete_result = mgr
			.lock()
			.unwrap_or_else(|e| e.into_inner())
			.delete(&id)
			.map_err(|e| e.to_string());

		match delete_result {
			Ok(()) => {
				let _ = spec::delete_spec_protocol(&id);
				audit_event(
					&auditor, &ctx, "delete", &id, &del_name, &del_ns, true, None,
				);

				if args.purge {
					if let Some(cfg_dir) = app_log_dir {
						if let Ok(base_log_dir) = paths::get_log_dir(&cfg_dir) {
							let app_log = base_log_dir.join(&id);
							purge_log_dir(&app_log);
						}
					}
				}

				let creds_dir = std::path::PathBuf::from(paths::CREDS_DIR).join(&id);
				let _ = std::fs::remove_dir_all(&creds_dir);

				let resp = serde_json::json!({"status": "deleted", "id": id});
				marshal(&resp)
			}
			Err(e) => {
				audit_event(
					&auditor,
					&ctx,
					"delete",
					&id,
					&del_name,
					&del_ns,
					false,
					Some(&e),
				);
				Err(e)
			}
		}
	}
}

/// `scale` — multi-process. Audit records `base_name` as the target so
/// forensics can answer "who tried to scale what".
pub fn scale_handler(
	mgr: SharedManager,
	auditor: std::sync::Arc<Logger>,
) -> impl Fn(RequestContext, RawMessage) -> Result<RawMessage, String> + Send + Sync + 'static {
	move |ctx, params| {
		let args: ScaleArgs =
			jsonx::unmarshal(params.as_bytes()).map_err(|e| format!("ERR_BAD_REQUEST: {e}"))?;
		if args.target < 0 {
			let msg = "ERR_BAD_REQUEST: target count must be >= 0";
			audit_event(
				&auditor,
				&ctx,
				"scale",
				&args.name,
				&args.name,
				&args.namespace,
				false,
				Some(msg),
			);
			return Err(msg.into());
		}

		let result = {
			let mut guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
			guard
				.scale(&args.namespace, &args.name, args.target as usize)
				.map_err(|e| e.to_string())
		};

		match result {
			Ok(resp) => {
				audit_event(
					&auditor,
					&ctx,
					"scale",
					&args.name,
					&args.name,
					&args.namespace,
					true,
					None,
				);
				marshal(&resp)
			}
			Err(e) => {
				audit_event(
					&auditor,
					&ctx,
					"scale",
					&args.name,
					&args.name,
					&args.namespace,
					false,
					Some(&e),
				);
				Err(e)
			}
		}
	}
}

/// Path-contained removal of an app log directory. Resolves symlinks on
/// both parent and target, then checks `target` is inside `parent` — the
/// same TOCTOU-safe two-step the Go code uses.
fn purge_log_dir(app_log_dir: &Path) {
	let Some(parent) = app_log_dir.parent() else {
		return;
	};
	let Ok(base_resolved) = std::fs::canonicalize(parent) else {
		return;
	};
	let Ok(target_resolved) = std::fs::canonicalize(app_log_dir) else {
		return;
	};
	if paths::within_root(&base_resolved, &target_resolved) {
		let _ = std::fs::remove_dir_all(&target_resolved);
	}
}
