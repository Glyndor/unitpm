//! The `reset` command.
//!
//! 11 cases ported from `internal/cli/commands/reset/cmd_test.go`.
//!
//! Resets a process's `Restarts` counter to zero. The process itself
//! is left running. Shares the same wildcard expansion as `delete`,
//! `flush`, and `reload`.

use std::io::Write;

use crate::cli::batch::Report;
use crate::cli::errs::UsageError;
use crate::cli::expand::{self, ExpandError, ListClient, ListError};
use crate::cli::help::CommandSpec;
use crate::ipc::transport::{Client, IPCClient, TransportError};
use crate::term;
use crate::types::ProcessInfo;

/// Dyn-compatible IPC surface for the `reset` command.
pub trait Ipc {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError>;
	fn reset(&mut self, id: &str) -> Result<ResetResponse, TransportError>;
}

/// Response shape returned by the daemon for a `reset` call.
#[derive(Debug, Clone, Default)]
pub struct ResetResponse {
	pub id: String,
	pub status: String,
}

/// Adapter for [`expand::targets`].
pub struct IpcList<'a>(pub &'a mut Box<dyn Ipc>);

impl<'a> ListClient for IpcList<'a> {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, ListError> {
		(**self.0)
			.list()
			.map_err(|e| ListError::Protocol(e.to_string()))
	}
}

/// Production adapter.
pub struct RealIpc(pub Client);

impl Ipc for RealIpc {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError> {
		self.0.list()
	}

	fn reset(&mut self, id: &str) -> Result<ResetResponse, TransportError> {
		self.0.reset(id)
	}
}

impl Ipc for Client {
	fn list(&mut self) -> Result<Vec<ProcessInfo>, TransportError> {
		self.call("list", None::<&()>, Some(&mut Vec::<ProcessInfo>::new()))?;
		let mut out: Vec<ProcessInfo> = Vec::new();
		self.call("list", None::<&()>, Some(&mut out))?;
		Ok(out)
	}

	fn reset(&mut self, id: &str) -> Result<ResetResponse, TransportError> {
		let body = serde_json::json!({"id": id});
		let mut val: serde_json::Value = serde_json::json!({});
		self.call("reset", Some(&body), Some(&mut val))?;
		let mut resp = ResetResponse::default();
		if let serde_json::Value::Object(map) = val {
			if let Some(s) = map.get("status").and_then(|v| v.as_str()) {
				resp.status = s.to_string();
			}
			if let Some(s) = map.get("id").and_then(|v| v.as_str()) {
				resp.id = s.to_string();
			}
		}
		Ok(resp)
	}
}

/// Run the `reset` command.
pub fn run<W: Write>(
	mut client: Option<Box<dyn Ipc>>,
	w: &mut W,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	let mut json_out = false;
	let mut namespace = String::new();
	let mut positional: Vec<String> = Vec::new();

	let mut i = 0;
	while i < args.len() {
		let a = &args[i];
		match a.as_str() {
			"--json" => json_out = true,
			"--namespace" => {
				if i + 1 >= args.len() {
					return Err(usage("missing value for --namespace".to_string()));
				}
				namespace = args[i + 1].clone();
				i += 1;
			}
			s if s.starts_with("--namespace=") => {
				namespace = s.trim_start_matches("--namespace=").to_string();
			}
			_ => positional.push(a.clone()),
		}
		i += 1;
	}

	if positional.is_empty() && namespace.is_empty() {
		return Err(usage("missing process ID or name".to_string()));
	}

	if client.is_none() {
		let c = Client::new()?;
		client = Some(Box::new(RealIpc(c)));
	}
	let client = client
		.as_mut()
		.expect("client either provided or opened above");

	let ids = expand_targets(client, &positional, &namespace)?;

	let mut rep = Report::new("reset");
	for id in &ids {
		match client.reset(id) {
			Ok(resp) => {
				if !json_out {
					let _ = writeln!(
						w,
						"{} Reset {}",
						term::green(format_args!("{}", "✓")),
						resp.id
					);
				}
				rep.ok(&resp.id, Default::default());
			}
			Err(e) => {
				if !json_out {
					let _ = writeln!(
						w,
						"{} Failed to reset {}: {}",
						term::red(format_args!("{}", "✗")),
						id,
						e
					);
				}
				rep.fail(id, Some(&e));
			}
		}
	}

	if json_out {
		rep.emit_json_to(w).map_err(json_err)?;
	} else if rep.summary.total > 1 {
		rep.print_summary(w).map_err(json_err)?;
	}

	rep.err().map_or(Ok(()), |e| Err(Box::new(e)))
}

fn json_err(e: std::io::Error) -> Box<dyn std::error::Error> {
	Box::new(e)
}

fn usage(msg: String) -> Box<dyn std::error::Error> {
	Box::new(UsageError::new(msg))
}

fn expand_targets(
	client: &mut Box<dyn Ipc>,
	ids: &[String],
	namespace: &str,
) -> Result<Vec<String>, ExpandError> {
	let mut list = IpcList(client);
	expand::targets::<IpcList<'_>>(Some(&mut list), ids, namespace)
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "reset".to_string(),
		aliases: Vec::new(),
		usage: "unitpm reset <id|name|ns:*|*>... [--namespace <ns>] [--json]".to_string(),
		description: "Reset a process's Restarts counter to zero".to_string(),
		options: vec![
			crate::cli::help::Option {
				short: String::new(),
				long: "--namespace <ns>".to_string(),
				description: "Reset every process in this namespace.".to_string(),
			},
			crate::cli::help::Option {
				short: String::new(),
				long: "--json".to_string(),
				description: "Emit a machine-readable batch report.".to_string(),
			},
			crate::cli::help::Option {
				short: "-h".to_string(),
				long: "--help".to_string(),
				description: "Show this help message.".to_string(),
			},
		],
		examples: vec![
			"unitpm reset api".to_string(),
			"unitpm reset prod:worker".to_string(),
			"unitpm reset 'prod:*'        # every process in namespace prod (quote the glob)"
				.to_string(),
			"unitpm reset --namespace prod # equivalent, no shell quoting needed".to_string(),
			"unitpm reset api worker --json | jq '.summary'".to_string(),
		],
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
