//! Audit-event emission for the handler layer.
//!
//! Mirrors `auditEvent` and `processMeta` from
//! `internal/daemon/handlers/handlers.go`. Every destructive handler funnels
//! its outcome through [`audit_event`] so removing a call turns a test red.
//!
//! The destination is an [`audit::Logger`](crate::daemon::audit::Logger)
//! that is either enabled (system mode → `/var/log/glyndor/unitpm/audit.log`)
//! or disabled (user mode). The helper is a no-op against a disabled logger.

use crate::daemon::audit::{Event, Logger};
use crate::daemon::handlers::service::SharedManager;
use crate::ipc::transport::{Identity, RequestContext};
use crate::types::ProcessState;

/// Emit one audit record. The caller has already collected `target`, `name`,
/// `ns` and the success/error state — this helper just adds the peer identity
/// from `ctx` and writes the JSONL line.
///
/// `name` and `ns` may be empty (e.g. on early failures before the process
/// exists).
#[allow(clippy::too_many_arguments)]
pub fn audit_event(
	logger: &Logger,
	ctx: &RequestContext,
	action: &str,
	target: &str,
	name: &str,
	ns: &str,
	success: bool,
	err: Option<&str>,
) {
	let mut event = Event::now(action, target);
	fill_identity(&mut event, &ctx.identity);
	event.name = name.to_string();
	event.ns = ns.to_string();
	event.success = success;
	if let Some(e) = err {
		event.error = e.to_string();
	}
	logger.log(event);
}

fn fill_identity(event: &mut Event, identity: &Identity) {
	if identity.uid.is_empty() && identity.gid.is_empty() && identity.pid == 0 {
		return;
	}
	event.uid = identity.uid.clone();
	event.gid = identity.gid.clone();
	event.pid = Some(identity.pid);
}

/// Best-effort snapshot of a process's name and namespace for audit
/// enrichment. Returns empty strings when the process has already left the
/// manager (e.g. after a delete completes).
pub fn process_meta(mgr: &SharedManager, id: &str) -> (String, String) {
	let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
	let Some(proc) = guard.get(id) else {
		return (String::new(), String::new());
	};
	let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
	let info = p.info();
	(info.name, info.namespace)
}

/// `true` when `state` indicates the process was actively running at the
/// moment we sampled it. Mirrors the Go `wasRunning := state in
/// {Running, Restarting, Online}` check that backs the `was_running` field
/// in the `stop` response.
#[must_use]
pub fn was_running(state: ProcessState) -> bool {
	matches!(
		state,
		ProcessState::Running | ProcessState::Restarting | ProcessState::Online
	)
}
