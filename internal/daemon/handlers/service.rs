//! Validation logic for `start` requests.
//!
//! Mirrors `internal/daemon/handlers/service.go`. The
//! [`start_process`] entry point is what [`start_handler`] calls into;
//! every other function in this file is its supporting cast.
//!
//! The `privileged` flag is the daemon's own mode — `true` for the system
//! instance (root or `unitpm` user), `false` for a user-mode one. The
//! `policy::authorize_start` check refuses `shell` execution and gates
//! `run_as=dynamic` on it, so this is a security parameter and not a
//! configuration one.

use std::path::{Component, Path, PathBuf};
use std::sync::{Arc, Mutex};

use crate::daemon::manager::Manager;
use crate::daemon::policy;
use crate::ipc::protocol::{AppResources, AppSpec, AppStop};
use crate::ipc::transport::Identity;
use crate::types::ProcessInfo;

/// Shared, mutable handle to a [`Manager`]. Every handler that mutates the
/// manager (start / stop / delete / restart / reload / reset / scale) takes
/// this; nothing else locks it.
pub type SharedManager = Arc<Mutex<Manager>>;

const NAME_MAX: usize = 128;
const NAMESPACE_MAX: usize = 64;
const EXEC_MAX: usize = 4096;
const ARG_MAX: usize = 4096;
const ARGS_LIMIT: usize = 256;
const CRON_MAX: usize = 256;
const ENV_KEY_MAX: usize = 256;
const ENV_VAL_MAX: usize = 8192;
const ENV_LIMIT: usize = 128;
const LOG_PATH_MAX: usize = 4096;
const CWD_MAX: usize = 4096;
const ENV_FILE_MAX: usize = 4096;

/// Validate `spec`, run the policy gate, and start the process.
pub fn start_process(
	mgr: &SharedManager,
	spec: AppSpec,
	identity: &Identity,
	daemon_privileged: bool,
) -> Result<ProcessInfo, String> {
	validate_spec(&spec)?;
	policy::authorize_start(&spec, identity, daemon_privileged).map_err(|e| e.to_string())?;

	let mut spec = spec;
	if let Some(env_file) = spec.env_file.as_deref() {
		if !env_file.is_empty() {
			let resolved = validate_env_file(env_file, identity)?;
			spec.env_file = Some(resolved);
		}
	}

	if let Some(cwd) = spec.cwd.as_deref() {
		if !cwd.is_empty() {
			spec.cwd = Some(validate_cwd(cwd)?);
		} else {
			spec.cwd = None;
		}
	}

	let mut guard = mgr.lock().unwrap_or_else(|e| e.into_inner());
	guard.start_with_spec(spec).map_err(|e| e.to_string())
}

/// `name` regex: human-friendly labels with letters, digits, spaces,
/// dots, underscores, hyphens, and a small set of shell-safe punctuation
/// (`:`, `#`, `@`, `!`, `,`, `(`, `)`, `+`, `=`, `&`). 128 chars max.
fn name_regex_valid(s: &str) -> bool {
	if s.is_empty() || s.len() > NAME_MAX {
		return false;
	}
	let bytes = s.as_bytes();
	if !bytes[0].is_ascii_alphanumeric() {
		return false;
	}
	s.chars().all(|c| {
		c.is_ascii_alphanumeric()
			|| matches!(
				c,
				' ' | '.' | '_' | ':' | '#' | '@' | '!' | ',' | '(' | ')' | '+' | '=' | '&' | '-'
			)
	})
}

/// `namespace` regex: letters, digits, dot, underscore, hyphen. 64 chars
/// max. Stricter than the name regex because `ns:name` parsing has to be
/// unambiguous.
fn namespace_regex_valid(s: &str) -> bool {
	if s.is_empty() || s.len() > NAMESPACE_MAX {
		return false;
	}
	let bytes = s.as_bytes();
	if !bytes[0].is_ascii_alphanumeric() {
		return false;
	}
	s.chars()
		.all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '_' | '-'))
}

fn validate_spec(spec: &AppSpec) -> Result<(), String> {
	if spec.exec.kind.is_empty() {
		return Err("ERR_BAD_REQUEST: exec type is required".into());
	}

	match spec.exec.kind.as_str() {
		"command" => {
			let cmd = spec.exec.command.as_deref().unwrap_or("");
			if cmd.is_empty() {
				return Err("ERR_BAD_REQUEST: command is required".into());
			}
			if cmd.len() > EXEC_MAX {
				return Err("ERR_LIMITS: command too long".into());
			}
		}
		"entry" => {
			if spec.exec.entry.as_deref().unwrap_or("").is_empty() {
				return Err("ERR_BAD_REQUEST: entry file is required".into());
			}
		}
		_ => return Err("ERR_BAD_REQUEST: invalid exec type".into()),
	}

	let args = spec.exec.args.as_deref();
	if args.map(|a| a.len()).unwrap_or(0) > ARGS_LIMIT {
		return Err("ERR_LIMITS: too many arguments".into());
	}
	if let Some(args) = args {
		for a in args {
			if a.len() > ARG_MAX {
				return Err("ERR_LIMITS: argument too long".into());
			}
		}
	}

	if !spec.name.is_empty() && !name_regex_valid(&spec.name) {
		return Err("ERR_BAD_REQUEST: invalid name format".into());
	}

	let ns = spec.namespace.as_deref().unwrap_or("");
	if !ns.is_empty() && !namespace_regex_valid(ns) {
		return Err("ERR_BAD_REQUEST: invalid namespace format".into());
	}

	if let Some(logs) = &spec.logs {
		if logs.dir.as_deref().unwrap_or("").len() > LOG_PATH_MAX {
			return Err("ERR_LIMITS: log dir too long".into());
		}
		if !logs.mode.is_empty() && logs.mode != "inherit" && logs.mode != "file" {
			return Err("ERR_BAD_REQUEST: invalid logs mode".into());
		}
		if let Some(f) = logs.format.as_deref() {
			if !f.is_empty() && f != "plain" && f != "json" {
				return Err("ERR_BAD_REQUEST: invalid logs format".into());
			}
		}
		if let Some(t) = logs.timestamp.as_deref() {
			if !t.is_empty() && t != "none" && t != "rfc3339" && t != "unix" {
				return Err("ERR_BAD_REQUEST: invalid logs timestamp".into());
			}
		}
		for p in [
			logs.dir.as_deref().unwrap_or(""),
			logs.stdout.as_deref().unwrap_or(""),
			logs.stderr.as_deref().unwrap_or(""),
		] {
			if p.is_empty() {
				continue;
			}
			if p.len() > LOG_PATH_MAX {
				return Err("ERR_LIMITS: log path too long".into());
			}
			if path_has_parent_traversal(p) {
				return Err("ERR_BAD_REQUEST: log paths must not contain '..'".into());
			}
		}
		if is_absolute_path(logs.stdout.as_deref().unwrap_or("")) {
			return Err("ERR_BAD_REQUEST: logs.stdout must be a relative filename".into());
		}
		if is_absolute_path(logs.stderr.as_deref().unwrap_or("")) {
			return Err("ERR_BAD_REQUEST: logs.stderr must be a relative filename".into());
		}
	}

	if let Some(cron) = &spec.cron {
		if cron.len() > CRON_MAX {
			return Err("ERR_LIMITS: cron spec too long".into());
		}
		if cron.contains('\n') || cron.contains('\r') {
			return Err("ERR_BAD_REQUEST: invalid cron spec".into());
		}
	}

	let env = spec.env.as_ref();
	if env.map(|e| e.len()).unwrap_or(0) > ENV_LIMIT {
		return Err("ERR_LIMITS: too many environment variables".into());
	}
	if let Some(env) = env {
		for (k, v) in env {
			if k.len() > ENV_KEY_MAX {
				return Err("ERR_LIMITS: env key too long".into());
			}
			if v.len() > ENV_VAL_MAX {
				return Err("ERR_LIMITS: env value too long".into());
			}
		}
	}

	if let Some(stop) = &spec.stop {
		validate_stop(stop)?;
	}
	if let Some(resources) = &spec.resources {
		validate_resources(resources)?;
	}
	Ok(())
}

fn validate_stop(s: &AppStop) -> Result<(), String> {
	if let Some(sig) = &s.signal {
		if !sig.is_empty() {
			let allowed = [
				"SIGTERM", "SIGINT", "SIGHUP", "SIGQUIT", "SIGUSR1", "SIGUSR2",
			];
			if !allowed.contains(&sig.as_str()) {
				return Err(
					"ERR_BAD_REQUEST: invalid stop signal; allowed: SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2"
						.into(),
				);
			}
		}
	}
	if let Some(t) = s.timeout_ms {
		if t != 0 && !(1000..=300_000).contains(&t) {
			return Err(
				"ERR_LIMITS: stop.timeout_ms must be between 1000 and 300000 (1s to 5min)".into(),
			);
		}
	}
	Ok(())
}

fn validate_resources(r: &AppResources) -> Result<(), String> {
	if let Some(m) = r.memory_max_bytes {
		if m < 0 {
			return Err("ERR_BAD_REQUEST: resources.memory_max_bytes must be >= 0".into());
		}
		if m != 0 && m < 1024 * 1024 {
			return Err("ERR_LIMITS: resources.memory_max_bytes must be >= 1 MiB when set".into());
		}
	}
	if let Some(c) = r.cpu_max_percent {
		if !(0..=10_000).contains(&c) {
			return Err("ERR_LIMITS: resources.cpu_max_percent must be between 0 and 10000".into());
		}
	}
	if let Some(t) = r.tasks_max {
		if t < 0 {
			return Err("ERR_BAD_REQUEST: resources.tasks_max must be >= 0".into());
		}
	}
	Ok(())
}

fn validate_env_file(path: &str, identity: &Identity) -> Result<String, String> {
	if path.len() > ENV_FILE_MAX {
		return Err("ERR_LIMITS: env_file path too long".into());
	}
	if path_has_parent_traversal(path) {
		return Err("ERR_BAD_REQUEST: env_file must not contain '..'".into());
	}
	if !is_absolute_path(path) {
		return Ok(path.to_string());
	}
	let resolved =
		std::fs::canonicalize(path).map_err(|_| "ERR_BAD_REQUEST: env_file not accessible")?;
	let info =
		std::fs::metadata(&resolved).map_err(|_| "ERR_BAD_REQUEST: env_file not accessible")?;
	if !info.file_type().is_file() {
		return Err("ERR_BAD_REQUEST: env_file must be a regular file".into());
	}

	if identity.uid.is_empty() {
		return Ok(resolved.to_string_lossy().to_string());
	}
	let caller_uid: u32 = identity
		.uid
		.parse()
		.map_err(|_| "ERR_BAD_REQUEST: env_file: caller identity invalid")?;
	if caller_uid == 0 {
		return Ok(resolved.to_string_lossy().to_string());
	}

	#[cfg(unix)]
	{
		use std::os::unix::fs::MetadataExt;
		let owner = info.uid();
		if owner != caller_uid {
			return Err("ERR_BAD_REQUEST: env_file not owned by caller".into());
		}
	}
	#[cfg(not(unix))]
	{
		let _ = caller_uid;
		return Err("ERR_INTERNAL: cannot stat env_file".into());
	}

	Ok(resolved.to_string_lossy().to_string())
}

fn validate_cwd(cwd: &str) -> Result<String, String> {
	if cwd.len() > CWD_MAX {
		return Err("ERR_LIMITS: cwd too long".into());
	}
	let abs = if is_absolute_path(cwd) {
		PathBuf::from(cwd)
	} else {
		std::fs::canonicalize(cwd).map_err(|_| "ERR_BAD_REQUEST: invalid cwd")?
	};
	let resolved = std::fs::canonicalize(&abs).map_err(|_| "ERR_BAD_REQUEST: invalid cwd")?;
	let restricted = ["/etc", "/proc", "/sys", "/boot", "/dev", "/run"];
	for r in restricted {
		if resolved == Path::new(r) || resolved.starts_with(format!("{r}/")) {
			return Err(
				"ERR_BAD_REQUEST: cwd is a restricted system directory; use --cwd to set a different path".into(),
			);
		}
	}
	let f = std::fs::File::open(&resolved).map_err(|_| {
		"ERR_BAD_REQUEST: cwd is not accessible to the daemon user; pass --cwd to a directory readable by the daemon (e.g. /var/lib/glyndor/unitpm or /tmp)"
			.to_string()
	})?;
	let info = f.metadata().map_err(|_| "ERR_BAD_REQUEST: invalid cwd")?;
	drop(f);
	if !info.is_dir() {
		return Err("ERR_BAD_REQUEST: invalid cwd".into());
	}
	Ok(resolved.to_string_lossy().to_string())
}

fn path_has_parent_traversal(p: &str) -> bool {
	let mut components = Path::new(p).components();
	matches!(components.next(), Some(Component::ParentDir))
}

fn is_absolute_path(p: &str) -> bool {
	Path::new(p).is_absolute()
}
