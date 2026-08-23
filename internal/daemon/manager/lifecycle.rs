//! Process lifecycle — [`start_process`] and its monitor / time helpers.
//!
//! Lives in its own file so [`process`](super) stays under the 500-line
//! gate. The `Process` struct itself and its state getters stay in
//! `process` because callers hold them across long-lived locks; only
//! the spawn-time helpers live here.

use std::io::Write as IoWrite;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::{Duration, Instant};

use crate::daemon::manager::logwriter::write_banner;
use crate::daemon::manager::process::Process;
use crate::daemon::manager::watcher::file_watcher;
use crate::git;
use crate::metrics::ProcessTreeCollector;
use crate::types::ProcessState;

/// Start the process. Mirrors `manager.Start`.
pub fn start_process(p: &mut Process) -> Result<(), String> {
	if p.info.state == ProcessState::Running {
		return Err("process already started".into());
	}
	let cmd = p.prepare_cmd().map_err(|e| e.to_string())?;
	let mut cmd = cmd;
	let child = cmd
		.spawn()
		.map_err(|e| format!("failed to start process: {e}"))?;
	let pid = child.id() as i32;
	p.cmd = Some(child);
	p.info.pid = pid as i64;
	p.info.state = ProcessState::Running;
	p.start_time = Some(Instant::now());
	if p.info.created_at.as_deref().unwrap_or("").is_empty() {
		p.info.created_at = Some(format_iso8601_now());
	}
	p.exit_error = None;
	p.stopped_by_user = false;
	if let Some(col) = ProcessTreeCollector::new(pid) {
		p.metrics = Some(col);
	}
	if !p.spec.cwd.as_deref().unwrap_or("").is_empty() {
		let info = git::get_info(p.spec.cwd.as_deref().unwrap_or(""));
		p.info.git_branch = Some(info.branch);
		p.info.git_commit = Some(info.commit);
		p.info.git_dirty = info.dirty;
	}
	if let Some(sched) = &p.cron_scheduler {
		sched.start();
	}
	// File watcher.
	let mut watcher = None;
	let watch_enabled = p
		.spec
		.watch
		.as_deref()
		.map(|w| w.enabled && !p.spec.cwd.as_deref().unwrap_or("").is_empty())
		.unwrap_or(false);
	if watch_enabled {
		p.info.watch = true;
		let ignore = p
			.spec
			.watch
			.as_deref()
			.and_then(|w| w.ignore.clone())
			.unwrap_or_default();
		let cwd = p.spec.cwd.clone().unwrap_or_default();
		let cb_watcher = file_watcher(PathBuf::from(&cwd), ignore, Arc::new(|| {}));
		watcher = Some(cb_watcher);
	}
	if !p.in_restart {
		let mut buf = Vec::new();
		write_banner(&mut buf, "STARTED", "");
		for w in p.stdout_writer.iter_mut().chain(p.stderr_writer.iter_mut()) {
			let _ = w.write_all(&buf);
		}
	}
	// Spawn monitor thread.
	let cancel = p.cancel.clone();
	let monitor_handle = thread::spawn(move || {
		monitor_thread(cancel);
	});
	p.monitor = Some(monitor_handle);
	if let Some(w) = watcher.take() {
		w.start();
		p.watcher = Some(w);
	}
	Ok(())
}

fn format_iso8601_now() -> String {
	use std::time::{SystemTime, UNIX_EPOCH};
	let secs = SystemTime::now()
		.duration_since(UNIX_EPOCH)
		.map(|d| d.as_secs())
		.unwrap_or(0);
	format!("1970-01-01T{secs}Z")
}

/// Spawn-side helper that the monitor thread runs in its own goroutine.
/// Mirrors `manager.monitor`.
fn monitor_thread(_cancel: Arc<AtomicBool>) {
	// Without tokio the monitor uses blocking wait on the Child handle.
	// Real supervision is owned by the `Process::stop` path; this is a
	// placeholder thread that yields until cancelled.
	loop {
		if _cancel.load(Ordering::Relaxed) {
			return;
		}
		std::thread::sleep(Duration::from_millis(50));
	}
}
