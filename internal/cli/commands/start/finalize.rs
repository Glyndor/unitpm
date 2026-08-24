//! Turning parsed flags into an `AppSpec`.
//!
//! Split out of `parser.rs`, which held two jobs in one file: reading the
//! command line, and assembling the spec that every downstream component —
//! the manager, the sandbox, the systemd wrapper — then consumes. The org's
//! limit is 500 lines and the file was 518.

use std::path::Path;

use super::memory::parse_memory_size;
use super::parser::SpecParser;
use crate::cli::errs::UsageError;
use crate::ipc::protocol::{
	AppExec, AppLogs, AppResources, AppRestart, AppSpec, AppStop, AppWatch, RunAsPolicy,
};
use crate::types::DEFAULT_NAMESPACE;

impl SpecParser {
	/// Build the actual [`AppSpec`] from the parsed fields. Pulled out
	/// so tests can drive it directly.
	pub fn finalize_spec(&self) -> Result<AppSpec, UsageError> {
		if self.cmd_parts.is_empty() {
			return Err(UsageError::new("missing command or entry file"));
		}

		let cwd = if self.cwd.is_empty() {
			std::env::current_dir()
				.map_err(|e| UsageError::new(format!("failed to get current directory: {e}")))?
		} else {
			std::path::PathBuf::from(&self.cwd)
		};
		let cwd = std::path::absolute(&cwd).map_err(|e| {
			UsageError::new(format!("failed to resolve absolute path for cwd: {e}"))
		})?;
		let cwd_str = cwd.to_string_lossy().to_string();

		let ns = if self.namespace.is_empty() {
			DEFAULT_NAMESPACE.to_string()
		} else {
			self.namespace.clone()
		};

		let mut spec = AppSpec {
			version: 1,
			id: String::new(),
			name: self.name.clone(),
			namespace: Some(ns),
			cwd: Some(cwd_str),
			cron: if self.cron.is_empty() {
				None
			} else {
				Some(self.cron.clone())
			},
			logs: Some(Box::new(AppLogs {
				mode: self.stdio.clone(),
				dir: if self.log_dir.is_empty() {
					None
				} else {
					Some(self.log_dir.clone())
				},
				stdout: if self.stdout_path.is_empty() {
					None
				} else {
					Some(self.stdout_path.clone())
				},
				stderr: if self.stderr_path.is_empty() {
					None
				} else {
					Some(self.stderr_path.clone())
				},
				format: if self.log_format.is_empty() {
					None
				} else {
					Some(self.log_format.clone())
				},
				timestamp: if self.log_timestamp.is_empty() {
					None
				} else {
					Some(self.log_timestamp.clone())
				},
			})),
			restart: Some(Box::new(AppRestart {
				policy: self.restart_policy.clone(),
				max_retries: Some(self.max_restarts),
				backoff_ms: Some(self.restart_delay),
				backoff_type: Some(self.backoff.clone()),
				stop_on_exit: Some(self.stop_on_exit.clone()),
			})),
			run_as: Some(Box::new(RunAsPolicy {
				mode: self.run_as.clone(),
			})),
			env: Some(std::collections::BTreeMap::new()),
			env_file: if self.env_file.is_empty() {
				None
			} else {
				Some(self.env_file.clone())
			},
			stop: None,
			resources: None,
			watch: None,
			created_at: None,
			disabled: false,
			exec: AppExec {
				kind: String::new(),
				command: None,
				args: None,
				entry: None,
				runtime: None,
				shell: self.shell,
			},
		};

		if !self.stop_signal.is_empty() || self.stop_timeout_ms != 0 {
			spec.stop = Some(Box::new(AppStop {
				signal: if self.stop_signal.is_empty() {
					None
				} else {
					Some(self.stop_signal.clone())
				},
				timeout_ms: if self.stop_timeout_ms == 0 {
					None
				} else {
					Some(self.stop_timeout_ms)
				},
			}));
		}

		if self.watch {
			let mut ignore: Vec<String> = Vec::new();
			if !self.watch_ignore.is_empty() {
				for pat in self.watch_ignore.split(',') {
					let trimmed = pat.trim();
					if trimmed.is_empty() {
						continue;
					}
					if trimmed.contains("..") || Path::new(trimmed).is_absolute() {
						return Err(UsageError::new(format!(
							"invalid ignore pattern {trimmed:?}: must be relative, no '..'"
						)));
					}
					ignore.push(trimmed.to_string());
				}
				if ignore.len() > 100 {
					return Err(UsageError::new("too many ignore patterns (max 100)"));
				}
			}
			spec.watch = Some(Box::new(AppWatch {
				enabled: true,
				ignore: if ignore.is_empty() {
					None
				} else {
					Some(ignore)
				},
			}));
		}

		if !self.memory_max.is_empty() || self.cpu_max_pct != 0 || self.tasks_max != 0 {
			let mem_bytes = parse_memory_size(&self.memory_max).map_err(UsageError::new)?;
			spec.resources = Some(Box::new(AppResources {
				memory_max_bytes: if mem_bytes == 0 {
					None
				} else {
					Some(mem_bytes)
				},
				cpu_max_percent: if self.cpu_max_pct == 0 {
					None
				} else {
					Some(self.cpu_max_pct)
				},
				tasks_max: if self.tasks_max == 0 {
					None
				} else {
					Some(self.tasks_max)
				},
			}));
		}

		crate::cli::commands::start::exec::resolve_exec(
			&self.cmd_parts,
			&self.runtime,
			self.shell,
			&mut spec.exec,
		);

		Ok(spec)
	}
}
