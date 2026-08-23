//! The internal `_exec-env` wrapper.
//!
//! 6 cases ported from `internal/cli/commands/execenv/cmd_test.go`.
//!
//! This binary is invoked by the daemon under systemd's
//! `LoadCredential=` mechanism: env vars land in
//! `$CREDENTIALS_DIRECTORY/env`, and this wrapper sources them into the
//! process environment before `execve`-ing the user's command. Keeping
//! this in its own module hides the implementation detail from the
//! dispatcher — the help text does not advertise the command.

use std::collections::HashMap;
use std::fs;
use std::io::Write;
use std::os::unix::process::CommandExt;
use std::path::Path;
use std::process::Command;

use crate::cli::help::CommandSpec;
use crate::env;

/// Env var name systemd uses to point at the credentials staging dir.
const CREDENTIALS_DIR_ENV: &str = "CREDENTIALS_DIRECTORY";

/// Run the `_exec-env` command. On success this does not return: the
/// current process image is replaced via `execve`. Returns an error only
/// when something prevents the exec.
pub fn run<W: Write>(w: &mut W, args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
	let _ = w;
	if args.is_empty() {
		return Err(usage());
	}

	if let Ok(creds_dir) = std::env::var(CREDENTIALS_DIR_ENV) {
		if !creds_dir.is_empty() {
			let env_path = Path::new(&creds_dir).join("env");
			if let Err(e) = load_env(&env_path) {
				eprintln!("unitpm: warning: failed to load env from credentials: {e}");
			}
		}
	}

	let cmd_name = &args[0];
	let cmd_args = args;

	let path = which(cmd_name).ok_or_else(|| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!("command not found: {cmd_name}"))
	})?;

	let env: Vec<String> = std::env::vars().map(|(k, v)| format!("{k}={v}")).collect();
	let mut cmd = Command::new(&path);
	cmd.args(&cmd_args[1..]);
	cmd.env_clear();
	for kv in &env {
		if let Some((k, v)) = kv.split_once('=') {
			cmd.env(k, v);
		}
	}
	// `exec` replaces the process image. If it returns, surface the error.
	let err = cmd.exec();
	Err(Box::<dyn std::error::Error>::from(format!(
		"exec failed: {err}"
	)))
}

fn usage() -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from("usage: unitpm _exec-env <cmd> [args...]")
}

fn which(name: &str) -> Option<String> {
	if name.contains('/') {
		if Path::new(name).is_file() {
			return Some(name.to_string());
		}
		return None;
	}
	let path = std::env::var_os("PATH")?;
	for dir in std::env::split_paths(&path) {
		let candidate = dir.join(name);
		if candidate.is_file() {
			return Some(candidate.display().to_string());
		}
	}
	None
}

/// Read a `KEY=VALUE` env file and apply each entry to the process
/// environment. Pure helper, exposed so the tests can exercise it
/// without driving the whole exec path.
pub fn load_env(path: &Path) -> Result<(), EnvError> {
	let parsed = parse_env_file(path)?;
	for (k, v) in parsed {
		std::env::set_var(k, v);
	}
	Ok(())
}

/// Parse a credential `env` file into key/value pairs. Splits on the
/// first `=`, ignores full-line comments, and treats the value as-is.
pub fn parse_env_file(path: &Path) -> Result<HashMap<String, String>, EnvError> {
	let text = fs::read_to_string(path).map_err(EnvError::Io)?;
	Ok(env::parse_str(&text))
}

/// Errors raised by the env-loading helper.
#[derive(Debug)]
pub enum EnvError {
	Io(std::io::Error),
}

impl std::fmt::Display for EnvError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			EnvError::Io(e) => write!(f, "io error: {e}"),
		}
	}
}

impl std::error::Error for EnvError {}

/// Help block — `_exec-env` is hidden so this is mostly informational.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer. Hidden so it never shows
/// up in root help.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "_exec-env".to_string(),
		aliases: Vec::new(),
		usage: "unitpm _exec-env <cmd> [args...]".to_string(),
		description: "Internal wrapper for DynamicUser environment bridging".to_string(),
		options: Vec::new(),
		examples: Vec::new(),
		hidden: true,
	}
}

#[cfg(test)]
mod tests;
