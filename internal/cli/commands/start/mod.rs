//! `unitpm start` — parse a command line into an `AppSpec` and tell the
//! daemon to spawn it.
//!
//! 25 cases ported from `internal/cli/commands/start/{cmd_test,parser_test}.go`.
//!
//! `start` is by far the largest of the four lifecycle commands
//! because it builds the `AppSpec` field by field rather than
//! mutating an existing one. The defaults are scattered across the
//! parser (log mode "file", restart policy "on-failure", exponential
//! backoff, etc.) and the spec-builder (logs.mode, run_as.mode).
//! Important: defaults are *parser-side*, not Rust `Default::default` —
//! see [`parser::SpecParser::new`] for what each one is, and check
//! against the Go parser when in doubt.

mod dryrun;
mod exec;
mod lexer;
mod memory;
mod parser;
mod spec_cmd;
mod time;

#[cfg(test)]
mod tests;

use std::collections::BTreeMap;
use std::io::{self, Write};

use serde::Serialize;

use crate::cli::commands::list;
use crate::ipc::protocol::{Request, StartRequest, StartResponseData};
use crate::ipc::transport::{Client, IPCClient};
use crate::spec;
use crate::term;
use crate::types::DEFAULT_NAMESPACE;

pub use memory::parse_memory_size;
pub use parser::{parse_app_spec, SpecParser};
pub use spec_cmd::spec;

use self::dryrun::print_dry_run;
use self::time::now_rfc3339;

/// One spawned instance, surfaced in both the `--json` batch report
/// and the post-action list highlight set.
#[derive(Debug, Clone, Serialize)]
pub struct StartedInstance {
	pub name: String,
	pub id: String,
	pub pid: i32,
	pub status: String,
	#[serde(skip_serializing_if = "Option::is_none")]
	pub namespace: Option<String>,
}

/// IPC surface the start command needs. The full transport::IPCClient
/// is used in production; tests implement the trait locally to keep
/// the generic `transport::IPCClient` out of the signature.
pub trait StartOps {
	type Error: std::fmt::Display;
	fn start(&mut self, req: &StartRequest) -> Result<StartResponseData, Self::Error>;
}

impl StartOps for Client {
	type Error = crate::ipc::transport::TransportError;

	fn start(&mut self, req: &StartRequest) -> Result<StartResponseData, Self::Error> {
		let mut resp_data: StartResponseData = StartResponseData {
			id: String::new(),
			proc_id: None,
			pid: None,
			status: None,
			message: None,
			created_at: None,
		};
		self.call("start", Some(req), Some(&mut resp_data))?;
		Ok(resp_data)
	}
}

/// Run the start command. `client` is created lazily after argument
/// validation so a bad invocation fails without touching the daemon
/// socket. Returns an error wrapping the underlying transport or
/// spec-construction failure.
pub fn run<O: Write, E: Write, C: StartOps + list::IpcOps>(
	client: Option<&mut C>,
	out: &mut O,
	err: &mut E,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if args_contain_help(args) {
		let _ = print_help_to(out);
		return Ok(());
	}

	let (dry_run, json_out, no_list, cmd_args) = filter_top_flags(args);

	let (app_spec, scale) = match parse_app_spec(&cmd_args) {
		Ok(pair) => pair,
		Err(e) => return Err(Box::new(e)),
	};

	let scale = if scale < 1 { 1 } else { scale };

	if dry_run {
		print_dry_run(out, &app_spec, scale, json_out)?;
		return Ok(());
	}

	let client = match client {
		Some(c) => c,
		None => {
			return Err(Box::<dyn std::error::Error + Send + Sync>::from(
				"start requires an IPC client (none provided)",
			));
		}
	};

	let mut auto_named = false;
	let base_name = if app_spec.name.is_empty() {
		auto_named = true;
		match app_spec.exec.kind.as_str() {
			"entry" => std::path::Path::new(app_spec.exec.entry.as_deref().unwrap_or(""))
				.file_name()
				.and_then(|s| s.to_str())
				.unwrap_or("")
				.to_string(),
			_ => std::path::Path::new(app_spec.exec.command.as_deref().unwrap_or(""))
				.file_name()
				.and_then(|s| s.to_str())
				.unwrap_or("")
				.to_string(),
		}
	} else {
		app_spec.name.clone()
	};

	let mut started: Vec<StartedInstance> = Vec::new();

	for i in 0..scale {
		let mut this_spec = app_spec.clone();

		let id = spec::generate_id();
		this_spec.id = id.clone();
		this_spec.created_at = Some(now_rfc3339());
		this_spec.namespace = Some(
			this_spec
				.namespace
				.clone()
				.unwrap_or_else(|| DEFAULT_NAMESPACE.to_string()),
		);

		this_spec.name = if auto_named {
			let short_id: String = id.chars().take(8).collect();
			if scale > 1 {
				format!("{base_name}-{}-{}", i + 1, short_id)
			} else {
				format!("{base_name}-{short_id}")
			}
		} else if scale > 1 {
			format!("{base_name}-{}", i + 1)
		} else {
			base_name.clone()
		};

		if this_spec.env.is_none() {
			this_spec.env = Some(BTreeMap::new());
		}
		if let Some(env) = this_spec.env.as_mut() {
			env.insert("UNITPM_INSTANCE".to_string(), i.to_string());
		}

		// Persist the spec before calling the daemon so it survives a
		// daemon crash mid-flight. The Go code uses XDG_CONFIG_HOME;
		// the spec module already resolves that.
		if let Err(e) = spec::save_spec_protocol(&id, &this_spec) {
			let msg = format!("failed to save spec: {e}");
			let _ = writeln!(err, "{}", term::red(format_args!("[unitpm][ERROR] {msg}")));
			return Err(Box::<dyn std::error::Error + Send + Sync>::from(msg));
		}

		let req = StartRequest {
			protocol_version: 1,
			kind: "start".to_string(),
			request_id: id.clone(),
			spec: this_spec.clone(),
		};

		match client.start(&req) {
			Ok(start_resp) => {
				let proc_id = start_resp.proc_id.clone().unwrap_or(id.clone());
				let pid = start_resp.pid.unwrap_or(0);
				let status = start_resp.status.clone().unwrap_or_default();
				started.push(StartedInstance {
					name: this_spec.name.clone(),
					id: proc_id.clone(),
					pid,
					status: status.clone(),
					namespace: this_spec.namespace.clone(),
				});
				if !json_out {
					print_success_response(out, &this_spec.name, &proc_id, pid, &status);
				}
			}
			Err(transport_err) => {
				// Roll back the persisted spec on IPC failure so we
				// don't leave a phantom entry behind.
				let _ = spec::delete_spec_protocol(&id);
				if json_out && !started.is_empty() {
					let partial = serde_json::json!({
						"partial": true,
						"started": started,
						"failed_at_instance": i + 1,
						"error": transport_err.to_string(),
					});
					let bytes = serde_json::to_vec(&partial)
						.map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })?;
					out.write_all(&bytes)?;
					writeln!(out)?;
				}
				let msg = format!("start failed for instance {}: {transport_err}", i + 1);
				return Err(Box::<dyn std::error::Error + Send + Sync>::from(msg));
			}
		}
	}

	finalize_start_inline(out, &started, scale, json_out, no_list, client);
	Ok(())
}

fn filter_top_flags(args: &[String]) -> (bool, bool, bool, Vec<String>) {
	let mut dry_run = false;
	let mut json_out = false;
	let mut no_list = false;
	let mut filtered: Vec<String> = Vec::with_capacity(args.len());
	for a in args {
		match a.as_str() {
			"--dry-run" | "-n" => dry_run = true,
			"--json" => json_out = true,
			"--no-list" => no_list = true,
			_ => filtered.push(a.clone()),
		}
	}
	(dry_run, json_out, no_list, filtered)
}

fn finalize_start_inline<O: Write, C: StartOps + list::IpcOps>(
	out: &mut O,
	started: &[StartedInstance],
	scale: i32,
	json_out: bool,
	no_list: bool,
	client: &mut C,
) {
	if json_out {
		let shape = serde_json::json!({
			"started": started,
			"count": started.len(),
		});
		if let Ok(bytes) = serde_json::to_vec(&shape) {
			let _ = out.write_all(&bytes);
			let _ = writeln!(out);
		}
		return;
	}

	if scale > 1 {
		let _ = writeln!(
			out,
			"\n{} Started {} instances",
			term::green(format_args!("{}", "✓")),
			started.len()
		);
	}

	if !no_list && !term::is_quiet() && !started.is_empty() {
		let highlight = started.iter().map(|s| s.id.clone()).collect();
		list::fetch_and_render(client, highlight, out);
	}
}

fn print_success_response(out: &mut impl Write, name: &str, proc_id: &str, pid: i32, status: &str) {
	let _ = writeln!(
		out,
		"{} Started {}",
		term::green(format_args!("{}", "✓")),
		name
	);
	if proc_id.len() > 8 {
		let short = proc_id.chars().take(8).collect::<String>();
		let _ = writeln!(out, "  ID:     {proc_id} (short: {short})");
	} else {
		let _ = writeln!(out, "  ID:     {proc_id}");
	}
	let _ = writeln!(out, "  PID:    {pid}");
	let _ = writeln!(out, "  Status: {status}");
}

fn args_contain_help(args: &[String]) -> bool {
	args.iter()
		.any(|a| a == "-h" || a == "--help" || a == "-help")
}

// --- spec / help -----------------------------------------------------------

/// Render the command-specific help to `out`.
pub fn print_help_to<W: Write>(w: &mut W) -> io::Result<()> {
	let spec = spec_cmd::spec();
	crate::cli::help::render_command_help(w, &spec)
}

/// Print the help block to stdout.
pub fn print_help() {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	let _ = print_help_to(&mut out);
}

// --- helpers used by tests -------------------------------------------------

/// Tiny helper used by tests that want a deterministic `Start` request.
#[allow(dead_code)]
pub(crate) fn make_request(id: &str, spec: crate::ipc::protocol::AppSpec) -> StartRequest {
	StartRequest {
		protocol_version: 1,
		kind: "start".to_string(),
		request_id: id.to_string(),
		spec,
	}
}

/// Helper used by tests to peek at the typed envelope.
#[allow(dead_code)]
pub(crate) fn dummy_request(id: &str) -> Request {
	Request {
		version: 1,
		id: id.to_string(),
		command: "start".to_string(),
		params: None,
	}
}

#[allow(unused_imports)]
use crate::ipc::protocol::RawMessage;
