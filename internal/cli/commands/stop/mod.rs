//! `unitpm stop` — terminate one or more managed processes via the daemon.
//!
//! 12 cases ported from `internal/cli/commands/stop/cmd_test.go`.
//!
//! The IPC surface this command needs is `list_processes` (for wildcard
//! expansion via [`cli::expand`]) plus `stop`. Defined locally so the
//! mock used in tests does not re-implement the full
//! `transport::IPCClient`, which is generic over `Serialize`/
//! `Deserialize` and not object-safe. Phase 6c/6d commands will each
//! bring their own — three private traits, not three variants of one
//! shared one.

use std::collections::HashSet;
use std::io::{self, Write};

#[cfg(test)]
mod tests;

use crate::cli::batch;
use crate::cli::commands::list;
use crate::cli::errs::UsageError;
use crate::cli::expand::{self, ListClient, ListError};
use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::term;
use crate::types::ProcessInfo;

/// Per-target outcome from the daemon's `stop` verb. The shape follows
/// the Go side: `{status, id, was_running}`.
#[derive(Debug, Clone, PartialEq)]
pub struct StopResponse {
	pub status: String,
	pub id: String,
	pub was_running: bool,
}

/// IPC surface the stop command needs from the IPC layer.
pub trait StopOps {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError>;
	fn stop(&mut self, id: &str) -> Result<StopResponse, IpcError>;
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

/// Run the stop command with the given IPC client. Writes per-target
/// progress to `out`, errors to `err`, and a trailing summary (or
/// `--json` batch report).
pub fn run<O: Write, E: Write, C: StopOps + list::IpcOps>(
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

	let mut report = batch::Report::new("stop");
	let mut touched: HashSet<String> = HashSet::with_capacity(ids.len());

	for id in &ids {
		match client.stop(id) {
			Ok(resp) => {
				let mut extra = std::collections::BTreeMap::new();
				extra.insert("was_running".into(), serde_json::json!(resp.was_running));
				if resp.was_running {
					if !parsed.json_out {
						let _ = writeln!(
							out,
							"{} Stopped {}",
							term::green(format_args!("{}", "✓")),
							resp.id
						);
					}
					report.ok(&resp.id, extra);
				} else if !parsed.json_out {
					let _ = writeln!(
						out,
						"{} Already stopped: {}",
						term::yellow(format_args!("{}", "!")),
						resp.id
					);
					report.noop(&resp.id, extra);
				} else {
					report.noop(&resp.id, extra);
				}
				touched.insert(resp.id);
			}
			Err(e) => {
				if !parsed.json_out {
					let _ = writeln!(
						err,
						"{} Failed to stop {}: {}",
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
	// Pre-split into flag tokens and positionals. The Go side uses
	// batch.SplitArgsWithValues so flags can appear either before or
	// after positional targets; we mirror that here.
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

fn expand_targets<C: StopOps + ?Sized>(
	client: &mut C,
	ids: &[String],
	namespace: &str,
) -> Result<Vec<String>, Box<dyn std::error::Error + Send + Sync>> {
	// Adapter: convert StopOps into something `expand::Targets` can
	// call via a closure-style wrap.
	struct ExpandWrap<'a, T: ?Sized> {
		inner: &'a mut T,
	}
	impl<T: StopOps + ?Sized> ListClient for ExpandWrap<'_, T> {
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

/// Internal: forward `StopOps` to anything that has the same surface,
/// via the `expand::ListClient` protocol for wildcard resolution.
trait _StopClientAdapter {}

fn args_contain_help(args: &[String]) -> bool {
	args.iter()
		.any(|a| a == "-h" || a == "--help" || a == "-help")
}

// --- spec / help -----------------------------------------------------------

/// Command spec for the registry.
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "stop".into(),
		aliases: Vec::new(),
		usage: "unitpm stop <id|name|ns:*|*>... [--namespace <ns>] [--json]".into(),
		description: "Stop a running process".into(),
		options: vec![
			HelpOption {
				short: "-h".into(),
				long: "--help".into(),
				description: "Show this help message.".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--namespace <ns>".into(),
				description: "Stop every process in this namespace.".into(),
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
			"unitpm stop api".into(),
			"unitpm stop prod:api".into(),
			"unitpm stop api worker-1 worker-2".into(),
			"unitpm stop 'prod:*'        # every process in namespace prod (quote the glob)".into(),
			"unitpm stop --namespace prod # equivalent, no shell quoting needed".into(),
			"unitpm stop api --json".into(),
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

/// Implementation of `StopOps` for the real `transport::Client`.
///
/// Lives in this module so the transport package stays unaware of the
/// command's surface — production callers wire `&mut transport::Client`
/// straight into `run(...)` and the trait resolves here.
mod wire {
	use super::{IpcError, StopResponse};
	use crate::ipc::transport::{Client, IPCClient};
	use crate::types::ProcessInfo;

	impl super::StopOps for Client {
		fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, IpcError> {
			let mut procs: Vec<ProcessInfo> = Vec::new();
			let params: Option<()> = None;
			self.call::<(), Vec<ProcessInfo>>("list", params.as_ref(), Some(&mut procs))
				.map_err(|e| IpcError(e.to_string()))?;
			Ok(procs)
		}

		fn stop(&mut self, id: &str) -> Result<StopResponse, IpcError> {
			let mut resp: serde_json::Value = serde_json::Value::Null;
			let params = serde_json::json!({ "id": id });
			self.call::<serde_json::Value, serde_json::Value>(
				"stop",
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
			let was_running = resp
				.get("was_running")
				.and_then(|v| v.as_bool())
				.unwrap_or(false);
			Ok(StopResponse {
				status,
				id: resp_id,
				was_running,
			})
		}
	}
}
