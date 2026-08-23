//! `unitpm restart` — restart one or more managed processes via the daemon.
//!
//! 10 cases ported from `internal/cli/commands/restart/cmd_test.go`.
//!
//! Structurally identical to `stop` minus the `was_running` /
//! `noop` branch — restart always treats success as `ok`. The Go side
//! re-implements the same flow; here we share no code with `stop`
//! because the brief tells us not to invent shared helpers between
//! commands in the same lane. Two private `IpcOps` traits are cheaper
//! than two variants of one shared one.

#[cfg(test)]
mod tests;

use std::collections::HashSet;
use std::io::{self, Write};

use crate::cli::batch;
use crate::cli::commands::list;
use crate::cli::errs::UsageError;
use crate::cli::expand::{self, ListClient, ListError};
use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::term;
use crate::types::ProcessInfo;

/// Per-target outcome from the daemon's `restart` verb.
#[derive(Debug, Clone, PartialEq)]
pub struct RestartResponse {
	pub status: String,
	pub id: String,
}

/// IPC surface the restart command needs from the IPC layer.
pub trait RestartOps {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError>;
	fn restart(&mut self, id: &str) -> Result<RestartResponse, IpcError>;
}

/// String-payload error wrapper.
#[derive(Debug)]
pub struct IpcError(pub String);

impl std::fmt::Display for IpcError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.write_str(&self.0)
	}
}

impl std::error::Error for IpcError {}

impl From<&str> for IpcError {
	fn from(s: &str) -> Self {
		Self(s.to_string())
	}
}

impl From<String> for IpcError {
	fn from(s: String) -> Self {
		Self(s)
	}
}

impl From<ListError> for IpcError {
	fn from(e: ListError) -> Self {
		Self(e.to_string())
	}
}

/// Run the restart command with the given IPC client. Writes
/// per-target progress to `out`, errors to `err`, and a trailing summary
/// (or `--json` batch report).
pub fn run<O: Write, E: Write, C: RestartOps + list::IpcOps>(
	client: &mut C,
	out: &mut O,
	err: &mut E,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if args_contain_help(args) {
		let _ = print_help_to(out);
		return Ok(());
	}

	let parsed = match parse_args(args) {
		Ok(p) => p,
		Err(e) => return Err(Box::new(e)),
	};

	if parsed.ids.is_empty() && parsed.namespace.is_empty() {
		return Err(Box::<dyn std::error::Error + Send + Sync>::from(
			"missing process ID or name",
		));
	}

	let ids = expand_targets(client, &parsed.ids, &parsed.namespace)?;

	let mut report = batch::Report::new("restart");
	let mut touched: HashSet<String> = HashSet::with_capacity(ids.len());

	for id in &ids {
		match client.restart(id) {
			Ok(resp) => {
				if !parsed.json_out {
					let _ = writeln!(
						out,
						"{} Restarted {}",
						term::green(format_args!("{}", "✓")),
						resp.id
					);
				}
				report.ok(&resp.id, std::collections::BTreeMap::new());
				touched.insert(resp.id);
			}
			Err(e) => {
				if !parsed.json_out {
					let _ = writeln!(
						err,
						"{} Failed to restart {}: {}",
						term::red(format_args!("{}", "✗")),
						id,
						e
					);
				}
				report.fail(id, Some(&e));
			}
		}
	}

	if parsed.json_out {
		report
			.emit_json_to(out)
			.map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })?;
		if let Some(be) = report.err() {
			return Err(Box::new(be));
		}
		return Ok(());
	}

	report
		.print_summary(out)
		.map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })?;
	if !parsed.no_list && !term::is_quiet() && !touched.is_empty() {
		list::fetch_and_render(client, touched, out);
	}
	if let Some(be) = report.err() {
		return Err(Box::new(be));
	}
	Ok(())
}

#[derive(Default, Debug)]
struct ParsedArgs {
	json_out: bool,
	no_list: bool,
	namespace: String,
	ids: Vec<String>,
}

fn parse_args(args: &[String]) -> Result<ParsedArgs, UsageError> {
	let mut p = ParsedArgs::default();
	let (flag_args, positionals) = batch::split_args_with_values(args, &["namespace".to_string()]);
	let mut i = 0;
	while i < flag_args.len() {
		let arg = flag_args[i].clone();
		if arg == "--json" {
			p.json_out = true;
		} else if arg == "--no-list" {
			p.no_list = true;
		} else if arg == "--namespace" {
			if i + 1 >= flag_args.len() {
				return Err(UsageError::new("missing value for --namespace"));
			}
			p.namespace = flag_args[i + 1].clone();
			i += 1;
		} else if let Some(v) = arg.strip_prefix("--namespace=") {
			p.namespace = v.to_string();
		} else if arg == "-h" || arg == "--help" {
			// Handled earlier.
		} else if arg.starts_with('-') {
			let name = arg.trim_start_matches('-');
			return Err(UsageError::new(format!("Unknown flag: -{name}")));
		}
		i += 1;
	}
	p.ids = positionals;
	Ok(p)
}

fn expand_targets<C: RestartOps + ?Sized>(
	client: &mut C,
	ids: &[String],
	namespace: &str,
) -> Result<Vec<String>, Box<dyn std::error::Error + Send + Sync>> {
	struct ExpandWrap<'a, T: ?Sized> {
		inner: &'a mut T,
	}
	impl<T: RestartOps + ?Sized> ListClient for ExpandWrap<'_, T> {
		fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, ListError> {
			self.inner
				.list_processes()
				.map_err(|e| ListError::Protocol(e.to_string()))
		}
	}
	let mut wrap = ExpandWrap { inner: client };
	expand::targets(Some(&mut wrap), ids, namespace)
		.map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })
}

fn args_contain_help(args: &[String]) -> bool {
	args.iter()
		.any(|a| a == "-h" || a == "--help" || a == "-help")
}

// --- spec / help -----------------------------------------------------------

/// Command spec for the registry.
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "restart".into(),
		aliases: Vec::new(),
		usage: "unitpm restart <id|name|ns:*|*>... [--namespace <ns>] [--json]".into(),
		description: "Restart a process".into(),
		options: vec![
			HelpOption {
				short: "-h".into(),
				long: "--help".into(),
				description: "Show this help message.".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--namespace <ns>".into(),
				description: "Restart every process in this namespace.".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--json".into(),
				description: "Emit a machine-readable batch report.".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--no-list".into(),
				description: "Skip the process list printed after the action.".into(),
			},
		],
		examples: vec![
			"unitpm restart api".into(),
			"unitpm restart prod:api worker".into(),
			"unitpm restart 'prod:*'        # every process in namespace prod (quote the glob)"
				.into(),
			"unitpm restart --namespace prod # equivalent, no shell quoting needed".into(),
			"unitpm restart api --json".into(),
		],
		hidden: false,
	}
}

/// Render the command-specific help to `out`.
pub fn print_help_to<W: Write>(w: &mut W) -> io::Result<()> {
	let spec = spec();
	crate::cli::help::render_command_help(w, &spec)
}

/// Print the help block to stdout.
pub fn print_help() {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	let _ = print_help_to(&mut out);
}

// --- Transport adapter ----------------------------------------------------

/// Implementation of `RestartOps` for the real `transport::Client`.
///
/// Lives in this module so the transport package stays unaware of the
/// command's surface — production callers wire `&mut transport::Client`
/// straight into `run(...)` and the trait resolves here.
mod wire {
	use super::{IpcError, RestartResponse};
	use crate::ipc::transport::{Client, IPCClient};
	use crate::types::ProcessInfo;

	impl super::RestartOps for Client {
		fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
			let mut procs: Vec<ProcessInfo> = Vec::new();
			let params: Option<()> = None;
			self.call::<(), Vec<ProcessInfo>>("list", params.as_ref(), Some(&mut procs))
				.map_err(|e| IpcError(e.to_string()))?;
			Ok(procs)
		}

		fn restart(&mut self, id: &str) -> Result<RestartResponse, IpcError> {
			let mut resp: serde_json::Value = serde_json::Value::Null;
			let params = serde_json::json!({ "id": id });
			self.call::<serde_json::Value, serde_json::Value>(
				"restart",
				Some(&params),
				Some(&mut resp),
			)
			.map_err(|e| IpcError(e.to_string()))?;
			let status = resp
				.get("status")
				.and_then(|v| v.as_str())
				.unwrap_or("")
				.to_string();
			let resp_id = resp
				.get("id")
				.and_then(|v| v.as_str())
				.unwrap_or("")
				.to_string();
			Ok(RestartResponse {
				status,
				id: resp_id,
			})
		}
	}
}
