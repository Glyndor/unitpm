//! Argument parser for the `start` command.
//!
//! Mirrors `internal/cli/commands/start/cmd.go` `specParser`. The
//! parser walks `args` once, separating flags from positional
//! "command parts" and producing a [`protocol::AppSpec`] ready for the
//! daemon's `start` IPC call.

use std::path::Path;

use crate::cli::errs::UsageError;
use crate::ipc::protocol::{
	AppExec, AppLogs, AppResources, AppRestart, AppSpec, AppStop, AppWatch, RunAsPolicy,
};
use crate::types::DEFAULT_NAMESPACE;

use super::memory::parse_memory_size;

/// Parse argv into an [`AppSpec`] and the `--scale` value. Stripped of
/// the top-level flags handled by the dispatcher (`--dry-run`,
/// `--json`, `--no-list`); the caller is expected to filter those out
/// first.
pub fn parse_app_spec(args: &[String]) -> Result<(AppSpec, i32), UsageError> {
	SpecParser::new(args).parse()
}

/// One field has a 2-char minimal default. The spec-parser owns the
/// parsing pass; the actual spec construction lives in the
/// [`SpecParser::finalize`] helper.
pub struct SpecParser {
	args: Vec<String>,
	pos: usize,

	name: String,
	namespace: String,
	cwd: String,
	stdio: String,
	run_as: String,
	cmd_parts: Vec<String>,
	cron: String,
	runtime: String,
	env_file: String,
	shell: bool,
	restart_policy: String,
	max_restarts: i32,
	restart_delay: i32,
	backoff: String,
	stop_on_exit: Vec<i32>,
	log_dir: String,
	stdout_path: String,
	stderr_path: String,
	log_format: String,
	log_timestamp: String,

	stop_signal: String,
	stop_timeout_ms: i32,

	memory_max: String,
	cpu_max_pct: i32,
	tasks_max: i32,

	watch: bool,
	watch_ignore: String,

	parsing_flags: bool,
	scale: i32,
}

impl SpecParser {
	pub fn new(args: &[String]) -> Self {
		Self {
			args: args.to_vec(),
			pos: 0,

			name: String::new(),
			namespace: String::new(),
			cwd: String::new(),
			stdio: "file".to_string(),
			run_as: "self".to_string(),
			cmd_parts: Vec::new(),
			cron: String::new(),
			runtime: String::new(),
			env_file: String::new(),
			shell: false,
			restart_policy: "on-failure".to_string(),
			max_restarts: 10,
			restart_delay: 2000,
			backoff: "expo".to_string(),
			stop_on_exit: vec![0],
			log_dir: String::new(),
			stdout_path: String::new(),
			stderr_path: String::new(),
			log_format: "plain".to_string(),
			log_timestamp: "rfc3339".to_string(),

			stop_signal: String::new(),
			stop_timeout_ms: 0,

			memory_max: String::new(),
			cpu_max_pct: 0,
			tasks_max: 0,

			watch: false,
			watch_ignore: String::new(),

			parsing_flags: true,
			scale: 1,
		}
	}

	pub fn parse(&mut self) -> Result<(AppSpec, i32), UsageError> {
		while self.pos < self.args.len() {
			let arg = self.args[self.pos].clone();

			if !self.parsing_flags {
				self.cmd_parts.push(arg.clone());
				self.pos += 1;
				continue;
			}

			if arg == "--" {
				self.pos += 1;
				while self.pos < self.args.len() {
					self.cmd_parts.push(self.args[self.pos].clone());
					self.pos += 1;
				}
				break;
			}

			if let Some(stripped) = arg.strip_prefix('-') {
				if stripped.is_empty() {
					self.cmd_parts.push(arg.clone());
					self.pos += 1;
					continue;
				}
				match self.handle_flag(&arg) {
					Ok(()) => {
						self.pos += 1;
						continue;
					}
					Err(e) => {
						if !self.cmd_parts.is_empty() {
							self.cmd_parts.push(arg.clone());
							self.pos += 1;
							continue;
						}
						return Err(UsageError::new(e));
					}
				}
			}

			self.cmd_parts.push(arg.clone());
			self.pos += 1;
		}

		let spec = self.finalize_spec()?;
		Ok((spec, self.scale))
	}

	fn handle_flag(&mut self, arg: &str) -> Result<(), String> {
		// Dispatch via two-step: choose the target field, then call the
		// read helper. Splitting the borrow avoids the re-borrow error
		// the match-on-`self` pattern produces.
		match arg {
			"--name" => {
				let mut target = std::mem::take(&mut self.name);
				let r = self.read_string_value(&mut target);
				self.name = target;
				r
			}
			"--namespace" => {
				let mut target = std::mem::take(&mut self.namespace);
				let r = self.read_string_value(&mut target);
				self.namespace = target;
				r
			}
			"--cwd" => {
				let mut target = std::mem::take(&mut self.cwd);
				let r = self.read_string_value(&mut target);
				self.cwd = target;
				r
			}
			"--cron" | "--schedule" => {
				let mut target = std::mem::take(&mut self.cron);
				let r = self.read_string_value(&mut target);
				self.cron = target;
				r
			}
			"--runtime" => {
				let mut target = std::mem::take(&mut self.runtime);
				let r = self.read_string_value(&mut target);
				self.runtime = target;
				r
			}
			"--isolation" => {
				let mut target = std::mem::take(&mut self.run_as);
				let r = self.read_string_value(&mut target);
				self.run_as = target;
				r
			}
			"--shell" => {
				self.shell = true;
				Ok(())
			}
			"--env-file" => {
				let mut target = std::mem::take(&mut self.env_file);
				let r = self.read_string_value(&mut target);
				self.env_file = target;
				r
			}
			"--restart" => {
				let mut target = std::mem::take(&mut self.restart_policy);
				let r = self.read_string_value(&mut target);
				self.restart_policy = target;
				r
			}
			"--max-restarts" => {
				let mut target = self.max_restarts;
				let r = self.read_int_value(&mut target);
				self.max_restarts = target;
				r
			}
			"--restart-delay" => {
				let mut target = self.restart_delay;
				let r = self.read_int_value(&mut target);
				self.restart_delay = target;
				r
			}
			"--backoff" => {
				let mut target = std::mem::take(&mut self.backoff);
				let r = self.read_string_value(&mut target);
				self.backoff = target;
				r
			}
			"--stop-on-exit" => {
				let mut target: Vec<i32> = std::mem::take(&mut self.stop_on_exit);
				let r = self.read_int_list(&mut target);
				self.stop_on_exit = target;
				r
			}
			"--log-dir" => {
				let mut target = std::mem::take(&mut self.log_dir);
				let r = self.read_string_value(&mut target);
				self.log_dir = target;
				r
			}
			"--stdout" => {
				let mut target = std::mem::take(&mut self.stdout_path);
				let r = self.read_string_value(&mut target);
				self.stdout_path = target;
				r
			}
			"--stderr" => {
				let mut target = std::mem::take(&mut self.stderr_path);
				let r = self.read_string_value(&mut target);
				self.stderr_path = target;
				r
			}
			"--log-format" => {
				let mut target = std::mem::take(&mut self.log_format);
				let r = self.read_string_value(&mut target);
				self.log_format = target;
				r
			}
			"--log-timestamp" => {
				let mut target = std::mem::take(&mut self.log_timestamp);
				let r = self.read_string_value(&mut target);
				self.log_timestamp = target;
				r
			}
			"--scale" | "--instances" => {
				let mut target = self.scale;
				let r = self.read_int_value(&mut target);
				self.scale = target;
				r
			}
			"--stop-signal" => {
				let mut target = std::mem::take(&mut self.stop_signal);
				let r = self.read_string_value(&mut target);
				self.stop_signal = target;
				r
			}
			"--stop-timeout" => {
				let mut target = self.stop_timeout_ms;
				let r = self.read_int_value(&mut target);
				self.stop_timeout_ms = target;
				r
			}
			"--watch" => {
				self.watch = true;
				Ok(())
			}
			"--watch-ignore" => {
				let mut target = std::mem::take(&mut self.watch_ignore);
				let r = self.read_string_value(&mut target);
				self.watch_ignore = target;
				r
			}
			"--memory-max" => {
				let mut target = std::mem::take(&mut self.memory_max);
				let r = self.read_string_value(&mut target);
				self.memory_max = target;
				r
			}
			"--cpu-max" => {
				let mut target = self.cpu_max_pct;
				let r = self.read_int_value(&mut target);
				self.cpu_max_pct = target;
				r
			}
			"--tasks-max" => {
				let mut target = self.tasks_max;
				let r = self.read_int_value(&mut target);
				self.tasks_max = target;
				r
			}
			_ => Err(format!("unknown flag: {arg}")),
		}
	}

	fn read_string_value(&mut self, target: &mut String) -> Result<(), String> {
		self.pos += 1;
		if self.pos >= self.args.len() {
			return Err("missing value for flag".to_string());
		}
		*target = self.args[self.pos].clone();
		Ok(())
	}

	fn read_int_value(&mut self, target: &mut i32) -> Result<(), String> {
		self.pos += 1;
		if self.pos >= self.args.len() {
			return Err("missing value for flag".to_string());
		}
		let v: i32 = self.args[self.pos]
			.parse()
			.map_err(|_| format!("invalid integer value: {}", self.args[self.pos]))?;
		*target = v;
		Ok(())
	}

	pub fn read_int_list(&mut self, target: &mut Vec<i32>) -> Result<(), String> {
		self.pos += 1;
		if self.pos >= self.args.len() {
			return Err("missing value for flag".to_string());
		}
		let mut out: Vec<i32> = Vec::new();
		for raw in self.args[self.pos].split(',') {
			let trimmed = raw.trim();
			if trimmed.is_empty() {
				continue;
			}
			let v: i32 = trimmed
				.parse()
				.map_err(|_| format!("invalid integer in list: {trimmed}"))?;
			out.push(v);
		}
		*target = out;
		Ok(())
	}

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

#[cfg(test)]
mod tests {
	use super::*;

	fn args(s: &[&str]) -> Vec<String> {
		s.iter().map(|t| (*t).to_string()).collect()
	}

	#[test]
	fn read_int_list_basic() {
		let mut p = SpecParser::new(&args(&["--cpus", "0,1,2"]));
		let mut result = Vec::new();
		p.read_int_list(&mut result).unwrap();
		assert_eq!(result, vec![0, 1, 2]);
	}

	#[test]
	fn read_int_list_single() {
		let mut p = SpecParser::new(&args(&["--cpus", "7"]));
		let mut result = Vec::new();
		p.read_int_list(&mut result).unwrap();
		assert_eq!(result, vec![7]);
	}

	#[test]
	fn read_int_list_with_spaces() {
		let mut p = SpecParser::new(&args(&["--cpus", "0, 1, 2"]));
		let mut result = Vec::new();
		p.read_int_list(&mut result).unwrap();
		assert_eq!(result.len(), 3);
	}

	#[test]
	fn read_int_list_missing_value_errors() {
		let mut p = SpecParser::new(&args(&["--cpus"]));
		let mut result = Vec::new();
		let r = p.read_int_list(&mut result);
		assert!(r.is_err(), "expected error");
	}

	#[test]
	fn read_int_list_invalid_int_errors() {
		let mut p = SpecParser::new(&args(&["--cpus", "0,abc,2"]));
		let mut result = Vec::new();
		let r = p.read_int_list(&mut result);
		assert!(r.is_err(), "expected error");
	}
}
