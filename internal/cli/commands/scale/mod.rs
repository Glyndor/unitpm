//! The `scale` command.
//!
//! 10 cases ported from `internal/cli/commands/scale/cmd_test.go`.
//!
//! Brings the number of instances of an app (name + namespace) to a
//! target count. The first positional argument is the app name (with
//! optional `<ns>:` prefix), the second is the integer target. JSON
//! output is the daemon's `ScaleResponse` rendered verbatim.

use std::io::Write;

use crate::cli::help::CommandSpec;
use crate::ipc::protocol::ScaleResponse;
use crate::ipc::transport::{Client, IPCClient, TransportError};
use crate::jsonx;
use crate::term;

/// Dyn-compatible IPC surface for the `scale` command.
pub trait Ipc {
	fn scale(
		&mut self,
		name: &str,
		namespace: &str,
		target: i32,
	) -> Result<ScaleResponse, TransportError>;
}

/// Production adapter.
pub struct RealIpc(pub Client);

impl Ipc for RealIpc {
	fn scale(
		&mut self,
		name: &str,
		namespace: &str,
		target: i32,
	) -> Result<ScaleResponse, TransportError> {
		self.0.scale(name, namespace, target)
	}
}

impl Ipc for Client {
	fn scale(
		&mut self,
		name: &str,
		namespace: &str,
		target: i32,
	) -> Result<ScaleResponse, TransportError> {
		let body = serde_json::json!({
			"name": name,
			"namespace": namespace,
			"target": target,
		});
		let mut resp = ScaleResponse {
			base_name: String::new(),
			namespace: String::new(),
			before: 0,
			after: 0,
			created: None,
			deleted: None,
		};
		self.call("scale", Some(&body), Some(&mut resp))?;
		Ok(resp)
	}
}

/// Run the `scale` command.
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
	let mut positional: Vec<String> = Vec::new();

	for a in args {
		match a.as_str() {
			"--json" => json_out = true,
			_ => positional.push(a.clone()),
		}
	}

	if positional.len() < 2 {
		return Err(usage("usage: unitpm scale <name> <N>".to_string()));
	}

	let name = &positional[0];
	let target_str = &positional[1];

	let mut ns = String::new();
	let mut base = name.as_str();
	if let Some((namespace, after)) = split_namespace(name) {
		ns = namespace.to_string();
		base = after;
	}

	let target: i32 = target_str
		.parse()
		.map_err(|_| -> Box<dyn std::error::Error> {
			Box::<dyn std::error::Error>::from(format!(
				"invalid target count {:?} (must be a non-negative integer)",
				target_str
			))
		})?;
	if target < 0 {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"invalid target count {:?} (must be a non-negative integer)",
			target_str
		)));
	}

	if client.is_none() {
		let c = Client::new()?;
		client = Some(Box::new(RealIpc(c)));
	}
	let client = client
		.as_mut()
		.expect("client either provided or opened above");

	let resp = client
		.scale(base, &ns, target)
		.map_err(|e| -> Box<dyn std::error::Error> {
			Box::new(std::io::Error::other(format!("scale failed: {e}")))
		})?;

	if json_out {
		let bytes = jsonx::marshal(&resp)?;
		w.write_all(&bytes)?;
		writeln!(w)?;
		return Ok(());
	}

	let _ = writeln!(
		w,
		"{} Scaled {}: {} → {}",
		term::green(format_args!("{}", "✓")),
		base,
		resp.before,
		resp.after
	);
	for c in resp.created.unwrap_or_default() {
		let _ = writeln!(w, "  {} {}", term::green(format_args!("{}", "+")), c);
	}
	for d in resp.deleted.unwrap_or_default() {
		let _ = writeln!(w, "  {} {}", term::red(format_args!("{}", "-")), d);
	}
	Ok(())
}

fn split_namespace(name: &str) -> Option<(&str, &str)> {
	let (before, after) = name.split_once(':')?;
	Some((before, after))
}

fn usage(msg: String) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(msg)
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "scale".to_string(),
		aliases: Vec::new(),
		usage: "unitpm scale <name> <N> [--json]".to_string(),
		description: "Scale an app up or down to the target number of instances".to_string(),
		options: vec![
			crate::cli::help::Option {
				short: String::new(),
				long: "--json".to_string(),
				description: "Emit the scale result as JSON on stdout.".to_string(),
			},
			crate::cli::help::Option {
				short: "-h".to_string(),
				long: "--help".to_string(),
				description: "Show this help message.".to_string(),
			},
		],
		examples: vec![
			"unitpm scale worker 5          # set 'worker' to exactly 5 instances".to_string(),
			"unitpm scale prod:api 10       # namespace-qualified".to_string(),
			"unitpm scale worker 0          # stop all instances (equivalent to delete all)"
				.to_string(),
			"unitpm scale worker 5 --json".to_string(),
		],
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
