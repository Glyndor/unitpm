//! Spawn — building the env, the resolved command line, and the log files
//! for a managed application. None of these helpers actually `exec`'s —
//! the supervisor (`supervise`) owns the `std::process::Command` and the
//! child handle.
//!
//! Mirrors the Go `prepareCmd`, `resolveCommand`, `prepareEnv`, and
//! `setupLogs` paths from `internal/daemon/manager/process.go`. The
//! hardened systemd-run wrapper lives in [`crate::daemon::manager::systemd`]
//! and is composed here by `build_command`.

use std::collections::HashMap;
use std::fs::{self, File, OpenOptions};
use std::io;
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::PathBuf;

use crate::daemon::manager::logwriter::timestamp_writer::{wrap_file, TimestampWriter};
use crate::daemon::manager::rotate::{current_rotate_config, rotate_if_large_cfg};
use crate::daemon::manager::systemd::{
	dynamic_command, DynamicCommand, DynamicContext, DynamicError,
};
use crate::env;
use crate::ipc::protocol::{AppExec, AppLogs, AppSpec, RunAsPolicy};
use crate::paths;

/// Errors that abort a `start`. Mirrors the Go error wrapping — the daemon
/// surfaces `ERR_BAD_REQUEST` / `ERR_LIMITS` / `ERR_UNSUPPORTED` prefixes
/// to the IPC layer.
#[derive(Debug)]
pub enum SpawnError {
	/// `ERR_BAD_REQUEST: invalid exec type` — unknown `AppExec.kind`.
	InvalidExecType,
	/// `ERR_BAD_REQUEST: entry and runtime required` — `kind=entry` but one is missing.
	EntryAndRuntimeRequired,
	/// `ERR_BAD_REQUEST: invalid runtime` — runtime string empty after split.
	InvalidRuntime,
	/// `ERR_LIMITS: too many arguments (max 256)`.
	TooManyArguments,
	/// `ERR_BAD_REQUEST: invalid cwd: <detail>`.
	InvalidCwd(io::Error),
	/// `ERR_BAD_REQUEST: failed to parse env file: <detail>`.
	EnvFileParse(String),
	/// `ERR_BAD_REQUEST: failed to open stdout log: <detail>`.
	OpenStdoutLog(String),
	/// `ERR_BAD_REQUEST: failed to open stderr log: <detail>`.
	OpenStderrLog(String),
	/// `ERR_BAD_REQUEST: failed to create log dir: <detail>`.
	CreateLogDir(String),
	/// `failed to write env creds: <detail>` — the dynamic-mode
	/// `LoadCredential` blob.
	DynamicEnvCreds(String),
	/// `failed to locate unitpm binary for env wrapper: <detail>`.
	DynamicNoBinary(String),
	/// `sandbox: locate unitpm binary: <detail>`.
	SandboxNoBinary(String),
	/// A delegated dynamic-mode build failure.
	Dynamic(DynamicError),
}

impl std::fmt::Display for SpawnError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			SpawnError::InvalidExecType => f.write_str("ERR_BAD_REQUEST: invalid exec type"),
			SpawnError::EntryAndRuntimeRequired => {
				f.write_str("ERR_BAD_REQUEST: entry and runtime required")
			}
			SpawnError::InvalidRuntime => f.write_str("ERR_BAD_REQUEST: invalid runtime"),
			SpawnError::TooManyArguments => f.write_str("ERR_LIMITS: too many arguments (max 256)"),
			SpawnError::InvalidCwd(e) => write!(f, "ERR_BAD_REQUEST: invalid cwd: {e}"),
			SpawnError::EnvFileParse(e) => {
				write!(f, "ERR_BAD_REQUEST: failed to parse env file: {e}")
			}
			SpawnError::OpenStdoutLog(e) => {
				write!(f, "ERR_BAD_REQUEST: failed to open stdout log: {e}")
			}
			SpawnError::OpenStderrLog(e) => {
				write!(f, "ERR_BAD_REQUEST: failed to open stderr log: {e}")
			}
			SpawnError::CreateLogDir(e) => {
				write!(f, "ERR_BAD_REQUEST: failed to create log dir: {e}")
			}
			SpawnError::DynamicEnvCreds(e) => write!(f, "failed to write env creds: {e}"),
			SpawnError::DynamicNoBinary(e) => {
				write!(f, "failed to locate unitpm binary for env wrapper: {e}")
			}
			SpawnError::SandboxNoBinary(e) => {
				write!(f, "sandbox: locate unitpm binary: {e}")
			}
			SpawnError::Dynamic(e) => write!(f, "dynamic: {e}"),
		}
	}
}

impl std::error::Error for SpawnError {}

impl From<DynamicError> for SpawnError {
	fn from(e: DynamicError) -> Self {
		SpawnError::Dynamic(e)
	}
}

/// Maximum number of arguments accepted in a `command` / `entry` exec.
const MAX_ARG_COUNT: usize = 256;

/// `sh -c` quoting: wrap each token in single quotes, escape embedded `'`
/// as `'\''`. Mirrors `manager.shellQuote` byte-for-byte.
#[must_use]
pub fn shell_quote(s: &str) -> String {
	let mut out = String::with_capacity(s.len() + 2);
	out.push('\'');
	for c in s.chars() {
		if c == '\'' {
			out.push_str("'\\''");
		} else {
			out.push(c);
		}
	}
	out.push('\'');
	out
}

/// Resolve the exec shape to `(binary, args)`. Mirrors
/// `manager.resolveCommand`.
pub fn resolve_command(spec: &AppSpec) -> Result<(String, Vec<String>), SpawnError> {
	let exec: &AppExec = &spec.exec;
	match exec.kind.as_str() {
		"command" => {
			let binary = exec
				.command
				.clone()
				.filter(|s| !s.is_empty())
				.ok_or(SpawnError::InvalidExecType)?;
			let args = exec.args.clone().unwrap_or_default();
			if args.len() > MAX_ARG_COUNT {
				return Err(SpawnError::TooManyArguments);
			}
			Ok((binary, args))
		}
		"entry" => {
			let entry = exec.entry.clone().filter(|s| !s.is_empty());
			let runtime = exec.runtime.clone().filter(|s| !s.is_empty());
			let (entry, runtime) = match (entry, runtime) {
				(Some(e), Some(r)) => (e, r),
				_ => return Err(SpawnError::EntryAndRuntimeRequired),
			};
			let rt_parts: Vec<&str> = runtime.split_whitespace().collect();
			if rt_parts.is_empty() {
				return Err(SpawnError::InvalidRuntime);
			}
			let mut final_args: Vec<String> = rt_parts[1..].iter().map(|s| s.to_string()).collect();
			final_args.push(entry);
			if let Some(args) = &exec.args {
				final_args.extend(args.iter().cloned());
			}
			if final_args.len() > MAX_ARG_COUNT {
				return Err(SpawnError::TooManyArguments);
			}
			Ok((rt_parts[0].to_string(), final_args))
		}
		_ => Err(SpawnError::InvalidExecType),
	}
}

/// Build the env vector for the child. Mirrors `manager.prepareEnv`:
/// system mode whitelists the inherited env; user mode forwards it. The
/// `HOME` entry is stripped in dynamic mode (systemd owns HOME) and
/// otherwise appended when missing. The spec's `EnvFile` and `Env` are
/// appended last.
pub fn prepare_env(spec: &AppSpec) -> Result<Vec<String>, SpawnError> {
	prepare_env_inner(spec)
}

/// Build the env vector for `spec` and return it.
fn prepare_env_inner(spec: &AppSpec) -> Result<Vec<String>, SpawnError> {
	let is_dynamic = matches!(spec.run_as, Some(ref r) if r.mode == "dynamic");

	let mut envs: Vec<String> = if paths::is_system_mode() {
		// System-mode whitelist — leaks nothing.
		const ALLOWED: &[&str] = &[
			"PATH",
			"LANG",
			"TERM",
			"TZ",
			"TMPDIR",
			"USER",
			"LOGNAME",
			"SHELL",
			"PWD",
			"XDG_DATA_HOME",
			"XDG_CONFIG_HOME",
			"XDG_STATE_HOME",
			"XDG_CACHE_HOME",
			"XDG_RUNTIME_DIR",
		];
		let mut out: Vec<String> = Vec::new();
		for entry in std::env::vars() {
			let key = entry.0;
			let mut allow = ALLOWED.contains(&key.as_str());
			if !allow && key.starts_with("LC_") {
				allow = true;
			}
			// Block loader variables even if somehow whitelisted.
			if key.starts_with("LD_") || key.starts_with("DYLD_") {
				allow = false;
			}
			if allow {
				out.push(format!("{key}={}", entry.1));
			}
		}
		out
	} else {
		std::env::vars().map(|(k, v)| format!("{k}={v}")).collect()
	};

	// HOME filter for dynamic mode; default-add HOME in non-dynamic when missing.
	let mut has_home = false;
	envs.retain(|e| {
		if let Some(rest) = e.strip_prefix("HOME=") {
			has_home = true;
			if is_dynamic {
				return false;
			}
			let _ = rest;
		}
		true
	});
	if !is_dynamic && !has_home {
		if let Ok(home) = std::env::var("HOME") {
			envs.push(format!("HOME={home}"));
		}
	}

	if let Some(path) = &spec.env_file {
		if !path.is_empty() {
			let parsed: HashMap<String, String> = match env::parse_file(path) {
				Ok(m) => m,
				Err(e) => return Err(SpawnError::EnvFileParse(e.to_string())),
			};
			for (k, v) in parsed {
				envs.push(format!("{k}={v}"));
			}
		}
	}
	if let Some(map) = &spec.env {
		for (k, v) in map {
			envs.push(format!("{k}={v}"));
		}
	}
	Ok(envs)
}

/// Result of [`setup_logs`]. Caller hands the writers to the spawned
/// command and the paths to the auto-restart path-reopen path.
pub struct LogSetup {
	/// Path to the stdout log file (may equal `stderr_path` for combined logs).
	pub stdout_path: PathBuf,
	/// Path to the stderr log file.
	pub stderr_path: PathBuf,
	/// Timestamp writer for stdout; `None` in inherit mode.
	pub stdout_writer: Option<TimestampWriter>,
	/// Timestamp writer for stderr; `None` in inherit mode.
	pub stderr_writer: Option<TimestampWriter>,
	/// Raw stdout `File` handle, kept alive for the duration of the
	/// child's lifetime.
	pub stdout_raw: Option<File>,
	/// Raw stderr `File` handle.
	pub stderr_raw: Option<File>,
	/// When `true`, the writer prepends a timestamp; when `false`, the
	/// child's writes go straight to the raw fd (perf-opt for high-rate
	/// log streams — the daemon-wide ticker still rotates the file).
	pub stdout_raw_fd: bool,
	/// Mirror of [`Self::stdout_raw_fd`] for stderr.
	pub stderr_raw_fd: bool,
}

/// Open the log files for `spec` and return handles. Mirrors
/// `manager.setupLogs`. Inherit mode yields `LogSetup { .. all None }`.
pub fn setup_logs(spec: &AppSpec, info_id: &str) -> Result<LogSetup, SpawnError> {
	let logs: AppLogs = spec.logs.clone().map(|b| *b).unwrap_or_else(|| AppLogs {
		mode: "inherit".into(),
		dir: None,
		stdout: None,
		stderr: None,
		format: None,
		timestamp: None,
	});

	if logs.mode == "inherit" {
		return Ok(LogSetup {
			stdout_path: PathBuf::new(),
			stderr_path: PathBuf::new(),
			stdout_writer: None,
			stderr_writer: None,
			stdout_raw: None,
			stderr_raw: None,
			stdout_raw_fd: false,
			stderr_raw_fd: false,
		});
	}

	let logs_dir = logs.dir.clone().unwrap_or_default();
	let stdout = logs.stdout.clone().unwrap_or_default();
	let stderr = logs.stderr.clone().unwrap_or_default();
	let (stdout_path, stderr_path) = paths::resolve_log_paths(info_id, &logs_dir, &stdout, &stderr)
		.map_err(|e| SpawnError::CreateLogDir(e.to_string()))?;

	if let Some(parent) = stdout_path.parent() {
		fs::create_dir_all(parent).map_err(|e| SpawnError::CreateLogDir(e.to_string()))?;
		fs::set_permissions(parent, fs::Permissions::from_mode(0o700)).ok();
	}
	if let Some(parent) = stderr_path.parent() {
		fs::create_dir_all(parent).map_err(|e| SpawnError::CreateLogDir(e.to_string()))?;
	}

	// Size-based rotation at Start time — the age trigger requires an
	// anchor that doesn't exist yet.
	let cfg = current_rotate_config();
	rotate_if_large_cfg(stdout_path.to_str().unwrap(), &cfg);
	if stderr_path != stdout_path {
		rotate_if_large_cfg(stderr_path.to_str().unwrap(), &cfg);
	}

	let raw_fd = logs.timestamp.as_deref() == Some("none");
	let log_flags = libc::O_APPEND | libc::O_CREAT | libc::O_WRONLY | libc::O_NOFOLLOW;

	let stdout_file = OpenOptions::new()
		.create(true)
		.append(true)
		.custom_flags(log_flags)
		.mode(0o600)
		.open(&stdout_path)
		.map_err(|e| SpawnError::OpenStdoutLog(e.to_string()))?;

	let cfg = current_rotate_config();
	let stdout_writer = wrap_file(
		stdout_file.try_clone().unwrap(),
		stdout_path.to_string_lossy().to_string(),
		cfg.clone(),
	);
	let stdout_writer = Some(stdout_writer);

	let stderr_writer;
	let stderr_file;
	if stderr_path == stdout_path {
		// Same path → same fd. We point both writers at the same
		// underlying writer. Cloning the `Option<TimestampWriter>` would
		// require `Clone` on the writer itself; instead, share via
		// `Arc`. (The TimestampWriter's `inner` is `Box<dyn Write>`, so
		// cloning the inner is not cheap; for the combined-log case,
		// the daemon only writes one stream anyway.)
		stderr_writer = None; // caller can detect "combined" via `stderr_path == stdout_path`
		stderr_file = stdout_file.try_clone().unwrap();
	} else {
		let f = OpenOptions::new()
			.create(true)
			.append(true)
			.custom_flags(log_flags)
			.mode(0o600)
			.open(&stderr_path)
			.map_err(|e| SpawnError::OpenStderrLog(e.to_string()))?;
		stderr_file = f.try_clone().unwrap();
		stderr_writer = Some(wrap_file(f, stderr_path.to_string_lossy().to_string(), cfg));
	}

	Ok(LogSetup {
		stdout_path,
		stderr_path,
		stdout_writer,
		stderr_writer,
		stdout_raw: Some(stdout_file),
		stderr_raw: Some(stderr_file),
		stdout_raw_fd: raw_fd,
		stderr_raw_fd: raw_fd,
	})
}

/// Build the dynamic-mode `systemd-run` argument list for inspection.
/// Pure function — does not spawn. Mirrors the argument list inside
/// `manager.prepareIsolation`'s dynamic branch.
#[must_use]
pub fn build_dynamic_args(spec: &AppSpec, info_id: &str, info_name: &str) -> Vec<String> {
	let ctx = DynamicContext {
		id: info_id,
		name: info_name,
		cwd: spec.cwd.as_deref().unwrap_or(""),
		resources: spec.resources.as_deref().cloned().map(Box::new),
	};
	dynamic_command(&ctx).args
}

/// Locate the daemon binary (`unitpm`) — used by both the sandbox wrapper
/// and the dynamic-mode `_exec-env` wrapper. Mirrors the Go-side helper
/// that resolves `unitpm` for the same purpose.
pub fn process_binary() -> Result<String, io::Error> {
	// 1. Prefer standard PATH lookup.
	if let Some(p) = scan_path_for("unitpm") {
		return Ok(p);
	}
	// 2. Fallback: adjacent to current binary.
	let exe = std::env::current_exe()?;
	let dir = exe
		.parent()
		.ok_or_else(|| io::Error::other("no parent dir for current_exe"))?;
	let candidate = dir.join("unitpm");
	if candidate.exists() {
		return Ok(candidate.to_string_lossy().to_string());
	}
	Err(io::Error::new(
		io::ErrorKind::NotFound,
		"unitpm binary not found in PATH or adjacent to daemon",
	))
}

fn scan_path_for(name: &str) -> Option<String> {
	let path = std::env::var_os("PATH")?;
	for entry in std::env::split_paths(&path) {
		let candidate = entry.join(name);
		if candidate.is_file() {
			return candidate.to_str().map(|s| s.to_string());
		}
	}
	None
}

/// Decide the isolation mode and assemble the appropriate wrapper.
#[derive(Debug)]
pub enum IsolationPlan {
	/// No wrapper — `std::process::Command` directly with `setpgid` enabled.
	Self_,
	/// `systemd-run` wrapper for dynamic mode. `DynamicCommand` is the
	/// pre-built argument list / binary / env.
	Dynamic(DynamicCommand),
	/// Sandbox wrapper: child exec'd via `_exec-sandbox`.
	Sandbox { binary: String },
}

pub fn plan_isolation(
	spec: &AppSpec,
	info_id: &str,
	info_name: &str,
	env: &[String],
) -> Result<IsolationPlan, SpawnError> {
	let run_as: RunAsPolicy = spec.run_as.clone().map(|b| *b).unwrap_or(RunAsPolicy {
		mode: "self".into(),
	});
	match run_as.mode.as_str() {
		"sandbox" => {
			let bin = process_binary().map_err(|e| SpawnError::SandboxNoBinary(e.to_string()))?;
			Ok(IsolationPlan::Sandbox { binary: bin })
		}
		"dynamic" => {
			// Write env to a LoadCredential file under creds_dir.
			let creds_dir: PathBuf = paths::CREDS_DIR.into();
			let creds = creds_dir.join(info_id);
			fs::create_dir_all(&creds).map_err(|e| SpawnError::DynamicEnvCreds(e.to_string()))?;
			let env_path = creds.join("env");
			let body = env.join("\n");
			fs::write(&env_path, &body).map_err(|e| SpawnError::DynamicEnvCreds(e.to_string()))?;
			fs::set_permissions(&env_path, fs::Permissions::from_mode(0o600)).ok();

			let bin = process_binary().map_err(|e| SpawnError::DynamicNoBinary(e.to_string()))?;
			let ctx = DynamicContext {
				id: info_id,
				name: info_name,
				cwd: spec.cwd.as_deref().unwrap_or(""),
				resources: spec.resources.as_deref().cloned().map(Box::new),
			};
			let dyn_cmd = dynamic_command(&ctx)
				.with_wrapper_binary(bin)
				.with_env_path(env_path.to_string_lossy().to_string());
			Ok(IsolationPlan::Dynamic(dyn_cmd))
		}
		_ => Ok(IsolationPlan::Self_),
	}
}

/// Pulled out so tests can call it.
#[cfg(test)]
pub(crate) fn env_for_test(spec: &AppSpec) -> Result<Vec<String>, SpawnError> {
	prepare_env_inner(spec)
}

#[cfg(test)]
#[path = "spawn_tests.rs"]
mod spawn_tests;
