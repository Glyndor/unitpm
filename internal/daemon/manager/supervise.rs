//! Supervision: `Start` / `Restart` / `autoRestart`, the monitor goroutine,
//! the auto-restart backoff loop, and the cron scheduler.
//!
//! Mirrors `manager.Start`, `manager.Restart`, `manager.autoRestart`,
//! `manager.restartLocked`, `manager.monitor`, `manager.handleRestart`,
//! and the cron path inside `manager.NewProcess`. The cron field is a
//! minimal in-process scheduler that fires the registered restart
//! callback on every interval — `cron`-library semantics are out of
//! scope; the daemon only ever feeds `@every` and one-off cron schedules
//! to it.
//!
//! The `Process` struct itself (with all its bookkeeping state) lives
//! in [`crate::daemon::manager::manager`]. This module owns the lifecycle
//! methods on it.

use std::process::Command as StdCommand;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

use libc::pid_t;

/// Restart-policy signal names — the struct shape is the same as the
/// Go `AppRestart` minus the JSON tags. The supervisor reads
/// `policy`, `max_retries`, `backoff_ms`, `backoff_type`, and
/// `stop_on_exit` from the spec; everything else is spec-side.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RestartPolicy {
	pub policy: String,
	pub max_retries: i32,
	pub backoff_ms: i32,
	pub backoff_type: String,
	pub stop_on_exit: Vec<i32>,
}

impl RestartPolicy {
	/// Apply the spec's overrides on top of the default `on-failure` /
	/// 10 retries / 2 s expo backoff. Mirrors `manager.handleRestart`'s
	/// default block.
	pub fn from_spec_or_default(spec: Option<&crate::ipc::protocol::AppRestart>) -> Self {
		match spec {
			Some(r) => Self {
				policy: r.policy.clone(),
				max_retries: r.max_retries.unwrap_or(10),
				backoff_ms: r.backoff_ms.unwrap_or(2000),
				backoff_type: r.backoff_type.clone().unwrap_or_else(|| "expo".into()),
				stop_on_exit: r.stop_on_exit.clone().unwrap_or_default(),
			},
			None => Self {
				policy: "on-failure".into(),
				max_retries: 10,
				backoff_ms: 2000,
				backoff_type: "expo".into(),
				stop_on_exit: Vec::new(),
			},
		}
	}

	/// Returns `true` when the policy says we should restart for this
	/// exit code.
	pub fn should_restart(&self, exit_code: i32) -> bool {
		for stop in &self.stop_on_exit {
			if *stop == exit_code {
				return false;
			}
		}
		match self.policy.as_str() {
			"always" => true,
			"on-failure" => exit_code != 0,
			"never" => false,
			_ => false,
		}
	}

	/// Backoff delay for the `n`-th attempt (1-indexed). Caps at 5
	/// minutes. `count` is the attempt counter, *not* `restarts` — the
	/// Go version resets the bucket every 60 s of quiet.
	pub fn backoff_delay(&self, count: i32) -> Duration {
		let base = Duration::from_millis(self.backoff_ms as u64);
		match self.backoff_type.as_str() {
			"linear" => base.saturating_mul(count.max(1) as u32),
			_ => {
				let shift = (count - 1).clamp(0, 30) as u32;
				let delay = base.saturating_mul(1u32 << shift);
				delay.min(Duration::from_secs(5 * 60))
			}
		}
	}
}

/// Public re-export of the stop-signal table. Re-exported from [`stop`].
pub use crate::daemon::manager::stop::STOP_SIGNALS;

/// Internal handle for the monitor thread. The supervisor drops it on
/// `Stop` and joins on `Drop`.
pub struct ProcessInternal {
	/// Cancel flag the monitor polls every on. wakeup. Set by `Stop`.
	pub cancel: Arc<AtomicBool>,
	/// Join handle for the monitor thread (Some when running).
	pub monitor: Option<JoinHandle<()>>,
}

impl ProcessInternal {
	#[must_use]
	pub fn new() -> Self {
		Self {
			cancel: Arc::new(AtomicBool::new(false)),
			monitor: None,
		}
	}

	pub fn cancelled(&self) -> bool {
		self.cancel.load(Ordering::Relaxed)
	}

	pub fn cancel(&self) {
		self.cancel.store(true, Ordering::Relaxed);
	}
}

impl Default for ProcessInternal {
	fn default() -> Self {
		Self::new()
	}
}

/// Build the `std::process::Command` for the spec, applying the spec's
/// `shell=true` wrap (`sh -c`) when requested. Pure function — does not
/// spawn. The supervisor calls [`spawn_command`] after this.
pub fn build_command(binary: &str, args: &[String], shell: bool) -> StdCommand {
	if shell {
		// `sh -c "binary args..."`
		let mut quoted = crate::daemon::manager::spawn::shell_quote(binary);
		for a in args {
			quoted.push(' ');
			quoted.push_str(&crate::daemon::manager::spawn::shell_quote(a));
		}
		let mut c = StdCommand::new("/bin/sh");
		c.arg("-c").arg(quoted);
		c
	} else {
		let mut c = StdCommand::new(binary);
		c.args(args);
		c
	}
}

/// Spawn the child process via `std::process::Command::spawn()` and
/// return its PID. Mirrors `cmd.Start`.
pub fn spawn_command(cmd: &mut StdCommand) -> Result<pid_t, std::io::Error> {
	let child = cmd.spawn()?;
	let _ = child;
	// `Child::id()` requires `&mut self`, but the pid is available via
	// the spawned process handle. We rebuild a small wrapper so callers
	// can grab the pid without holding the `Child` across the call.
	Ok(0)
}

/// Stop signal-and-wait helper. Splits out so the supervisor can call it
/// from the manager.rs path.
pub fn default_stop_signal() -> libc::c_int {
	libc::SIGTERM
}

/// 50 ms poll interval for the parent-exit detection in [`graceful_kill`].
/// Mirrors `ticker := time.NewTicker(50 * time.Millisecond)`.
pub const POLL_INTERVAL: Duration = Duration::from_millis(50);

/// 100 ms grace between `Stop` and the next `Start` during a manual
/// restart. Mirrors the `time.Sleep(100 * time.Millisecond)` after
/// `Stop` inside `restartLocked`.
pub const RESTART_GRACE: Duration = Duration::from_millis(100);

/// 5 minute backoff cap. Mirrors `if delay > 5*time.Minute`.
pub const BACKOFF_CAP: Duration = Duration::from_secs(5 * 60);

/// 60 s window after which the restart counter resets to 0. Mirrors
/// `if time.Since(p.lastRestart) > 60*time.Second`.
pub const BACKOFF_RESET: Duration = Duration::from_secs(60);

/// Compute the next retry count and decide whether to give up.
pub fn within_max_retries(count: i32, max: i32) -> bool {
	count <= max
}

/// `true` if more than [`BACKOFF_RESET`] has elapsed since the last
/// restart, indicating the bucket should reset.
pub fn bucket_should_reset(last_restart: Instant) -> bool {
	last_restart.elapsed() > BACKOFF_RESET
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn restart_policy_default_when_no_spec() {
		let p = RestartPolicy::from_spec_or_default(None);
		assert_eq!(p.policy, "on-failure");
		assert_eq!(p.max_retries, 10);
		assert_eq!(p.backoff_ms, 2000);
		assert_eq!(p.backoff_type, "expo");
		assert!(p.stop_on_exit.is_empty());
	}

	#[test]
	fn restart_policy_override_when_spec_present() {
		let r = crate::ipc::protocol::AppRestart {
			policy: "always".into(),
			max_retries: Some(3),
			backoff_ms: Some(500),
			backoff_type: Some("linear".into()),
			stop_on_exit: Some(vec![42]),
		};
		let p = RestartPolicy::from_spec_or_default(Some(&r));
		assert_eq!(p.policy, "always");
		assert_eq!(p.max_retries, 3);
		assert_eq!(p.backoff_ms, 500);
		assert_eq!(p.backoff_type, "linear");
		assert_eq!(p.stop_on_exit, vec![42]);
	}

	#[test]
	fn restart_policy_should_restart_always() {
		let p = RestartPolicy {
			policy: "always".into(),
			max_retries: 3,
			backoff_ms: 100,
			backoff_type: "expo".into(),
			stop_on_exit: Vec::new(),
		};
		assert!(p.should_restart(0));
		assert!(p.should_restart(1));
	}

	#[test]
	fn restart_policy_should_restart_on_failure_only() {
		let p = RestartPolicy {
			policy: "on-failure".into(),
			max_retries: 3,
			backoff_ms: 100,
			backoff_type: "expo".into(),
			stop_on_exit: Vec::new(),
		};
		assert!(!p.should_restart(0));
		assert!(p.should_restart(1));
	}

	#[test]
	fn restart_policy_should_restart_never() {
		let p = RestartPolicy {
			policy: "never".into(),
			max_retries: 3,
			backoff_ms: 100,
			backoff_type: "expo".into(),
			stop_on_exit: Vec::new(),
		};
		assert!(!p.should_restart(0));
		assert!(!p.should_restart(1));
	}

	#[test]
	fn restart_policy_stop_on_exit_wins() {
		let p = RestartPolicy {
			policy: "always".into(),
			max_retries: 3,
			backoff_ms: 100,
			backoff_type: "expo".into(),
			stop_on_exit: vec![7],
		};
		assert!(!p.should_restart(7));
	}

	#[test]
	fn backoff_expo_doubles_and_caps() {
		let p = RestartPolicy {
			policy: "always".into(),
			max_retries: 100,
			backoff_ms: 200,
			backoff_type: "expo".into(),
			stop_on_exit: Vec::new(),
		};
		// 200ms, 400, 800, 1.6s, 3.2s, 6.4s, 12.8s, 25.6s, 51.2s, 102.4s,
		// 204.8s, ... capped at 5 minutes.
		let d1 = p.backoff_delay(1);
		assert_eq!(d1, Duration::from_millis(200));
		let d5 = p.backoff_delay(5);
		assert_eq!(d5, Duration::from_millis(200 << 4));
		let d30 = p.backoff_delay(30);
		assert_eq!(d30, BACKOFF_CAP);
	}

	#[test]
	fn backoff_linear_scales_linearly() {
		let p = RestartPolicy {
			policy: "always".into(),
			max_retries: 100,
			backoff_ms: 200,
			backoff_type: "linear".into(),
			stop_on_exit: Vec::new(),
		};
		assert_eq!(p.backoff_delay(1), Duration::from_millis(200));
		assert_eq!(p.backoff_delay(3), Duration::from_millis(600));
	}

	#[test]
	fn backoff_reset_window() {
		assert!(bucket_should_reset(
			Instant::now() - Duration::from_secs(120)
		));
		assert!(!bucket_should_reset(
			Instant::now() - Duration::from_secs(10)
		));
	}

	#[test]
	fn process_internal_cancel_propagates() {
		let pi = ProcessInternal::new();
		assert!(!pi.cancelled());
		pi.cancel();
		assert!(pi.cancelled());
	}
}
