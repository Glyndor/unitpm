//! `flush` — truncate stdout/stderr for a process. Rejects symlinks and
//! any path that escapes the log root, and reports the number of bytes
//! reclaimed.
//!
//! Lives in its own file so [`super`](crate::daemon::handlers::control)
//! stays under the 500-line gate; the path-containment check, the
//! `symlink_metadata` rejection, and the audit-on-every-exit-path emit
//! all want space of their own.

use std::path::Path;

use crate::daemon::audit::Logger;
use crate::daemon::handlers::audit::{audit_event, process_meta};
use crate::daemon::handlers::query::{marshal, IdArgs};
use crate::daemon::handlers::service::SharedManager;
use crate::ipc::protocol::{AppSpec, RawMessage};
use crate::ipc::transport::RequestContext;
use crate::jsonx;
use crate::paths;
use crate::spec;

pub fn flush_handler(
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
					"flush",
					&args.id,
					"",
					"",
					false,
					Some(&e.to_string()),
				);
				return Err(e.to_string());
			}
		};
		let (flush_name, flush_ns) = process_meta(&mgr, &id);

		// Defer the audit emit so every exit path is captured.
		let auditor_for_defer = auditor.clone();
		let ctx_for_defer = ctx.clone();
		let defer_audit = |err: Option<&str>| {
			audit_event(
				&auditor_for_defer,
				&ctx_for_defer,
				"flush",
				&id,
				&flush_name,
				&flush_ns,
				err.is_none(),
				err,
			);
		};

		// Look up the spec — live process or on-disk.
		let s: AppSpec = {
			let guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
			if let Some(proc) = guard.get(&id) {
				let p = proc.lock().unwrap_or_else(|e| e.into_inner());
				p.spec_copy()
			} else {
				drop(guard);
				match spec::load_spec_protocol(&id) {
					Ok(s) => s,
					Err(_) => {
						let msg = format!("process not found: {}", args.id);
						defer_audit(Some(&msg));
						return Err(msg);
					}
				}
			}
		};

		let logs_dir = s
			.logs
			.as_ref()
			.and_then(|l| l.dir.clone())
			.unwrap_or_default();
		let stdout = s
			.logs
			.as_ref()
			.and_then(|l| l.stdout.clone())
			.unwrap_or_default();
		let stderr = s
			.logs
			.as_ref()
			.and_then(|l| l.stderr.clone())
			.unwrap_or_default();

		let (stdout_path, stderr_path) =
			match paths::resolve_log_paths(&s.id, &logs_dir, &stdout, &stderr) {
				Ok(v) => v,
				Err(e) => {
					let msg = format!("failed to resolve log paths: {e}");
					defer_audit(Some(&msg));
					return Err(msg);
				}
			};

		let log_root = if !logs_dir.is_empty() {
			match paths::get_log_dir(&logs_dir) {
				Ok(p) => p,
				Err(e) => {
					let msg = format!("failed to resolve log root: {e}");
					defer_audit(Some(&msg));
					return Err(msg);
				}
			}
		} else {
			match paths::get_log_dir("") {
				Ok(p) => p,
				Err(e) => {
					let msg = format!("failed to resolve log root: {e}");
					defer_audit(Some(&msg));
					return Err(msg);
				}
			}
		};

		let base_resolved = match std::fs::canonicalize(&log_root) {
			Ok(p) => p,
			Err(e) => {
				let msg = format!("failed to resolve log root symlinks: {e}");
				defer_audit(Some(&msg));
				return Err(msg);
			}
		};

		let mut bytes_freed: i64 = 0;
		let mut last_err: Option<String> = None;
		for p in [stdout_path, stderr_path] {
			if p.as_os_str().is_empty() {
				continue;
			}

			let mut target = p.clone();
			if !target.is_absolute() {
				target = log_root.join(&target);
			}
			let target = match strip_double_slash(&target) {
				Ok(t) => t,
				Err(msg) => {
					last_err = Some(msg);
					continue;
				}
			};

			let target_dir = match target.parent() {
				Some(d) => d.to_path_buf(),
				None => {
					last_err = Some("refusing to truncate log outside log root".into());
					continue;
				}
			};

			let target_resolved_dir = match std::fs::canonicalize(&target_dir) {
				Ok(r) => r,
				Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
					let dir_clean = clean_lexical(&target_dir);
					if !paths::within_root(&base_resolved, &dir_clean) {
						last_err = Some("refusing to truncate log outside log root".into());
						continue;
					}
					if !paths::within_root(&base_resolved, &target) {
						last_err = Some("refusing to truncate log outside log root".into());
						continue;
					}
					continue;
				}
				Err(e) => {
					last_err = Some(format!("failed to resolve log directory symlinks: {e}"));
					continue;
				}
			};

			if !paths::within_root(&base_resolved, &target_resolved_dir) {
				last_err = Some("refusing to truncate log outside log root".into());
				continue;
			}
			if !paths::within_root(&base_resolved, &target) {
				last_err = Some("refusing to truncate log outside log root".into());
				continue;
			}

			let info = match std::fs::symlink_metadata(&target) {
				Ok(m) => m,
				Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
				Err(e) => {
					last_err = Some(format!("failed to stat log file: {e}"));
					continue;
				}
			};

			if info.file_type().is_symlink() {
				last_err = Some("ERR_BAD_REQUEST: refusing to truncate symlink log file".into());
				continue;
			}
			if !info.file_type().is_file() {
				last_err = Some(format!(
					"refusing to truncate non-regular log file {}",
					target.display()
				));
				continue;
			}

			let size_before = info.len() as i64;
			if let Err(e) = std::fs::File::create(&target) {
				if e.kind() != std::io::ErrorKind::NotFound {
					last_err = Some(format!("failed to truncate {}: {e}", target.display()));
					continue;
				}
			} else {
				bytes_freed += size_before;
			}
		}

		if let Some(msg) = last_err {
			defer_audit(Some(&msg));
			return Err(msg);
		}

		defer_audit(None);
		let resp = serde_json::json!({
			"status": "flushed",
			"id": id,
			"bytes_freed": bytes_freed,
		});
		marshal(&resp)
	}
}

/// `Path::clean` analogue that rejects empty inputs (signals "the file
/// does not exist" through the caller).
fn strip_double_slash(p: &Path) -> Result<std::path::PathBuf, String> {
	if p.as_os_str().is_empty() {
		return Err("refusing to truncate log outside log root".into());
	}
	Ok(p.components().collect())
}

fn clean_lexical(p: &Path) -> std::path::PathBuf {
	p.components().collect()
}
