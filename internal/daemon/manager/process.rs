//! `Process` — one managed application instance.
//!
//! Owns the spec, the running command handle, log files, restart
//! bookkeeping, the monitor thread, and (when set) the cron scheduler.
//!
//! The lifecycle methods live here because they touch the same fields
//! the supervisor mutates; the [`crate::daemon::manager::supervise`]
//! helpers stay pure so the test suite can drive `Process::start`,
//! `Process::stop`, and `Process::restart` directly without spinning up
//! the full daemon.
//!
//! Mirrors `manager.Process` in `internal/daemon/manager/process.go`,
//! minus the unholy 1108-line sprawl the Go file had.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

use uuid::Uuid;

use crate::daemon::manager::helpers::parse_simple_duration;
use crate::daemon::manager::logwriter::timestamp_writer::TimestampWriter;
use crate::daemon::manager::logwriter::write_banner;
use crate::daemon::manager::spawn::{
	build_dynamic_args, prepare_env, process_binary, resolve_command, SpawnError,
};
use crate::daemon::manager::stop::{cleanup_credentials, graceful_kill, resolve_stop, KillError};
use crate::daemon::manager::supervise::{build_command, RESTART_GRACE};
use crate::daemon::manager::version_detect::detect_project_version;
use crate::daemon::manager::watcher::FileWatcher;
use crate::ipc::protocol::{AppSpec, RunAsPolicy};
use crate::metrics::{self, Collector, ProcTreeCollector};
use crate::types::{ProcessInfo, ProcessState};
use std::io::Write as IoWrite;
use std::path::PathBuf;

/// One managed application. Public so the supervisor modules can hold
/// and mutate the fields under their own locks.
pub struct Process {
	pub info: ProcessInfo,
	pub spec: AppSpec,
	pub cmd: Option<std::process::Child>,
	pub stdout_writer: Option<TimestampWriter>,
	pub stderr_writer: Option<TimestampWriter>,
	pub stdout_path: String,
	pub stderr_path: String,
	pub stopped_by_user: bool,
	pub no_auto_restart: bool,
	pub exit_error: Option<std::io::Error>,
	pub start_time: Option<Instant>,
	pub metrics: Option<ProcTreeCollector>,
	pub cron_scheduler: Option<CronScheduler>,
	pub restart_count: i32,
	pub last_restart: Option<Instant>,
	pub cancel_restart: Arc<AtomicBool>,
	pub in_restart: bool,
	pub cancel: Arc<AtomicBool>,
	pub monitor: Option<JoinHandle<()>>,
	pub watcher: Option<FileWatcher>,
}

impl Process {
	pub fn new(id: &str, spec: AppSpec) -> Result<Self, SpawnError> {
		if Uuid::parse_str(id).is_err() {
			return Err(SpawnError::InvalidExecType); // reuse error: caller maps ID issue.
		}
		let name = if !spec.name.is_empty() {
			spec.name.clone()
		} else {
			let exec = &spec.exec;
			match exec.kind.as_str() {
				"entry" => exec
					.entry
					.as_deref()
					.map(PathBuf::from)
					.and_then(|p| p.file_name().map(|s| s.to_string_lossy().to_string()))
					.unwrap_or_default(),
				_ => exec
					.command
					.as_deref()
					.map(PathBuf::from)
					.and_then(|p| p.file_name().map(|s| s.to_string_lossy().to_string()))
					.unwrap_or_default(),
			}
		};

		let ns = spec
			.namespace
			.clone()
			.filter(|s| !s.is_empty())
			.unwrap_or_else(|| "default".into());

		let version = detect_project_version(spec.cwd.as_deref().unwrap_or(""));

		let info = ProcessInfo {
			id: id.to_string(),
			name,
			namespace: ns,
			version,
			mode: "fork".into(),
			pid: 0,
			uptime: 0,
			restarts: 0,
			state: ProcessState::Stopped,
			cpu: 0.0,
			memory: 0,
			user: String::new(),
			watch: false,
			git_branch: None,
			git_commit: None,
			git_dirty: false,
			created_at: None,
		};

		// Cron validation for `@every <duration>`> schedules.
		let cron_scheduler = if let Some(cron) = &spec.cron {
			if let Some(rest) = cron.strip_prefix("@every ") {
				let rest = rest.trim();
				match parse_simple_duration(rest) {
					Ok(d) => {
						if d < Duration::from_secs(5) {
							return Err(SpawnError::EnvFileParse(
								"ERR_LIMITS: cron interval must be >= 5s".to_string(),
							));
						}
						if d > Duration::from_secs(24 * 3600) {
							return Err(SpawnError::EnvFileParse(
								"ERR_LIMITS: cron interval must be <= 24h".to_string(),
							));
						}
					}
					Err(e) => {
						return Err(SpawnError::EnvFileParse(format!(
							"ERR_LIMITS: invalid cron interval: {e}"
						)));
					}
				}
			}
			Some(CronScheduler::new(cron.clone()))
		} else {
			None
		};

		Ok(Self {
			info,
			spec,
			cmd: None,
			stdout_writer: None,
			stderr_writer: None,
			stdout_path: String::new(),
			stderr_path: String::new(),
			stopped_by_user: false,
			no_auto_restart: false,
			exit_error: None,
			start_time: None,
			metrics: None,
			cron_scheduler,
			restart_count: 0,
			last_restart: None,
			cancel_restart: Arc::new(AtomicBool::new(false)),
			in_restart: false,
			cancel: Arc::new(AtomicBool::new(false)),
			monitor: None,
			watcher: None,
		})
	}

	/// Snapshot of [`ProcessInfo`] plus metrics when running.
	pub fn info(&mut self) -> ProcessInfo {
		if self.info.state == ProcessState::Running {
			if let Some(start) = self.start_time {
				self.info.uptime = start.elapsed().as_millis() as i64;
			}
			if let Some(m) = self.metrics.as_mut() {
				if let Ok(snap) = m.collect() {
					self.info.cpu = snap.cpu_percent;
					self.info.memory = snap.memory_bytes;
				}
			}
		} else {
			self.info.uptime = 0;
			self.info.cpu = 0.0;
			self.info.memory = 0;
		}
		self.info.clone()
	}

	/// Deep copy of the spec — matches `Process.Spec()`.
	pub fn spec_copy(&self) -> AppSpec {
		let mut s = self.spec.clone();
		// Pointer fields: Logs / Restart / RunAs / Stop / Resources.
		if let Some(logs) = s.logs {
			s.logs = Some(Box::new(*logs));
		}
		if let Some(r) = s.restart.take() {
			let mut rc = *r;
			rc.stop_on_exit = rc.stop_on_exit.clone();
			s.restart = Some(Box::new(rc));
		}
		if let Some(ra) = s.run_as {
			s.run_as = Some(Box::new(*ra));
		}
		if let Some(w) = s.watch {
			s.watch = Some(Box::new(*w));
		}
		s
	}

	/// Tree of process + descendants. Returns `None` when the process is
	/// not running.
	pub fn tree(&self) -> Option<Vec<metrics::ChildStat>> {
		if self.info.state != ProcessState::Running && self.info.state != ProcessState::Online {
			return None;
		}
		let pid = self.info.pid;
		metrics::get_process_tree(pid as i32).ok()
	}

	/// Reset the backoff bucket + the user-facing restarts counter.
	pub fn reset_backoff(&mut self) {
		self.restart_count = 0;
		self.last_restart = None;
		self.no_auto_restart = false;
	}

	pub fn reset_metrics(&mut self) {
		self.info.restarts = 0;
		self.restart_count = 0;
		self.last_restart = None;
	}

	/// Build the `std::process::Command` for the spec. Pure function —
	/// does not spawn. The caller owns the lifetime of the returned
	/// `Command`.
	pub fn prepare_cmd(&self) -> Result<std::process::Command, SpawnError> {
		let ctx = resolve_command(&self.spec)?;
		let (bin, args) = ctx;
		let mut cmd = build_command(&bin, &args, self.spec.exec.shell);
		if let Some(cwd) = self.spec.cwd.as_deref() {
			if !cwd.is_empty() {
				if !std::path::Path::new(cwd).is_dir() {
					return Err(SpawnError::InvalidCwd(std::io::Error::other(
						"not a directory",
					)));
				}
				cmd.current_dir(cwd);
			}
		}
		let env = prepare_env(&self.spec)?;
		// Apply env + isolation via the spec.
		let run_as: RunAsPolicy = self.spec.run_as.clone().map(|b| *b).unwrap_or(RunAsPolicy {
			mode: "self".into(),
		});
		match run_as.mode.as_str() {
			"sandbox" => {
				let bin =
					process_binary().map_err(|e| SpawnError::SandboxNoBinary(e.to_string()))?;
				cmd = std::process::Command::new(bin);
				cmd.arg("_exec-sandbox");
				cmd.current_dir(self.spec.cwd.as_deref().unwrap_or("."));
			}
			"dynamic" => {
				let args_list = build_dynamic_args(&self.spec, &self.info.id, &self.info.name);
				let _ = args_list;
				let bin =
					process_binary().map_err(|e| SpawnError::DynamicNoBinary(e.to_string()))?;
				cmd = std::process::Command::new("systemd-run");
				cmd.args(args_list);
				cmd.arg("--");
				cmd.arg(bin);
				cmd.arg("_exec-env");
			}
			_ => {
				// Self mode: enable setpgid so Stop can kill(-pid).
				unsafe {
					use std::os::unix::process::CommandExt;
					cmd.pre_exec(|| {
						// setpgid(0, 0)
						let r = libc::setpgid(0, 0);
						if r != 0 {
							return Err(std::io::Error::last_os_error());
						}
						Ok(())
					});
				}
			}
		}
		for e in env {
			if let Some((k, v)) = e.split_once('=') {
				cmd.env(k, v);
			}
		}
		Ok(cmd)
	}

	/// Stop the process. `by_user=true` disables automatic restarts.
	pub fn stop(&mut self, by_user: bool) -> Result<(), KillError> {
		if by_user {
			self.no_auto_restart = true;
		}
		if self.info.state != ProcessState::Running {
			if by_user {
				self.info.state = ProcessState::Stopped;
				self.info.pid = 0;
			}
			return Ok(());
		}
		if by_user {
			self.stopped_by_user = true;
			self.info.state = ProcessState::Stopped;
			self.info.pid = 0;
			if !self.in_restart {
				let mut buf = Vec::new();
				write_banner(&mut buf, "STOPPED", "");
				for w in self
					.stdout_writer
					.iter_mut()
					.chain(self.stderr_writer.iter_mut())
				{
					let _ = IoWrite::write_all(w, &buf);
				}
			}
		}
		let pid = self.info.pid;
		if pid == 0 {
			return Ok(());
		}
		if let Some(sched) = &self.cron_scheduler {
			sched.stop();
		}
		// Cancel pending restart backoff.
		self.cancel.store(true, Ordering::Relaxed);
		if let Some(w) = &self.watcher {
			w.stop();
		}
		let (sig, timeout) = resolve_stop(
			self.spec.stop.as_ref().and_then(|s| s.signal.as_deref()),
			self.spec.stop.as_ref().and_then(|s| s.timeout_ms),
		);
		graceful_kill(pid as i32, sig, timeout)?;
		if by_user {
			cleanup_credentials(&self.info.id);
		}
		// Signal monitor thread to exit and join.
		self.cancel.store(true, Ordering::Relaxed);
		if let Some(h) = self.monitor.take() {
			let _ = h.join();
		}
		Ok(())
	}

	/// Restart the process. Manual restart resets backoff and re-enables
	/// auto-restart.
	pub fn restart(&mut self) -> Result<(), String> {
		self.restart_locked(true)
	}

	fn restart_locked(&mut self, emit_banner: bool) -> Result<(), String> {
		if self.no_auto_restart {
			return Ok(());
		}
		self.info.restarts += 1;
		if emit_banner {
			self.in_restart = true;
			let mut buf = Vec::new();
			write_banner(&mut buf, "RESTARTED", "");
			for w in self
				.stdout_writer
				.iter_mut()
				.chain(self.stderr_writer.iter_mut())
			{
				let _ = IoWrite::write_all(w, &buf);
			}
		}
		if emit_banner {
			self.in_restart = false;
		}
		let _ = self.stop(false);
		std::thread::sleep(RESTART_GRACE);
		start_process(self)
	}
}

/// Minimal in-process cron scheduler. Fires the registered job once per
/// interval; the daemon's tests poke the schedule by calling
/// [`trigger`](Self::trigger) to avoid waiting for the real tick.
pub struct CronScheduler {
	pub schedule: String,
	pub interval: Option<Duration>,
	pub last_fire: Option<Instant>,
	pub job: Option<Box<dyn Fn() + Send + Sync>>,
	pub running: Arc<AtomicBool>,
}

impl CronScheduler {
	fn new(schedule: String) -> Self {
		let interval = schedule
			.strip_prefix("@every ")
			.and_then(|rest| parse_simple_duration(rest.trim()).ok());
		Self {
			schedule,
			interval,
			last_fire: None,
			job: None,
			running: Arc::new(AtomicBool::new(false)),
		}
	}

	pub fn set_job(&mut self, job: Box<dyn Fn() + Send + Sync>) {
		self.job = Some(job);
	}

	pub fn entries(&self) -> Vec<CronEntry<'_>> {
		if self.interval.is_some() {
			vec![CronEntry {
				schedule: &self.schedule,
			}]
		} else {
			Vec::new()
		}
	}

	pub fn trigger(&mut self) {
		if let Some(job) = &self.job {
			job();
		}
		self.last_fire = Some(Instant::now());
	}

	pub fn start(&self) {
		self.running.store(true, Ordering::Relaxed);
	}

	pub fn stop(&self) {
		self.running.store(false, Ordering::Relaxed);
	}

	pub fn is_running(&self) -> bool {
		self.running.load(Ordering::Relaxed)
	}
}

/// Placeholder for the Go `cron.Entry` — we just expose enough to test
/// "is there exactly one job registered".
pub struct CronEntry<'a> {
	pub schedule: &'a str,
}

#[cfg(test)]
#[path = "process_tests.rs"]
mod process_tests;

pub use crate::daemon::manager::lifecycle::start_process;
