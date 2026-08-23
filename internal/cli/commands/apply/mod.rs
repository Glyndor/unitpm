//! The `apply` command.
//!
//! 8 cases ported from `internal/cli/commands/apply/cmd_test.go`.
//!
//! Reads a `unitpm.yml` manifest, validates it, generates a UUID v7 spec ID
//! for every app, saves each spec to disk, then forwards a `start` IPC
//! request to the daemon for each one. The batch report follows the same
//! JSON shape as the other multi-target commands (stop/restart/etc.).

use std::fs;
use std::io::Write;
use std::path::Path;

use serde_json::json;

use crate::cli::batch::Report;
use crate::cli::errs::UsageError;
use crate::cli::help::CommandSpec;
use crate::ipc::protocol::{AppSpec, StartRequest, StartResponse};
use crate::ipc::transport::{Client, IPCClient, TransportError};
use crate::manifest::{self, ToAppSpecs};
use crate::spec;
use crate::term;
use crate::types;

/// Dyn-compatible IPC surface for the `apply` command. The production
/// adapter wraps [`Client`]; tests use a recorder.
pub trait Ipc {
	/// Forward a `start` request and decode the response.
	fn start(&mut self, req: &StartRequest) -> Result<StartResponse, TransportError>;
}

/// Production adapter backed by [`Client`].
pub struct RealIpc(pub Client);

impl Ipc for RealIpc {
	fn start(&mut self, req: &StartRequest) -> Result<StartResponse, TransportError> {
		let mut resp = StartResponse {
			protocol_version: 0,
			kind: String::new(),
			request_id: String::new(),
			ok: false,
			data: None,
			error: None,
		};
		self.0.call("start", Some(req), Some(&mut resp))?;
		Ok(resp)
	}
}

impl Ipc for Client {
	fn start(&mut self, req: &StartRequest) -> Result<StartResponse, TransportError> {
		let mut resp = StartResponse {
			protocol_version: 0,
			kind: String::new(),
			request_id: String::new(),
			ok: false,
			data: None,
			error: None,
		};
		self.call("start", Some(req), Some(&mut resp))?;
		Ok(resp)
	}
}

/// Run the `apply` command.
pub fn run<W: Write>(
	mut client: Option<Box<dyn Ipc>>,
	w: &mut W,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	let json_out = args.iter().any(|a| a == "--json");
	let mut flag_args: Vec<&str> = Vec::new();
	let mut positional: Vec<String> = Vec::new();
	for a in args {
		if let Some(name) = a.strip_prefix("--") {
			flag_args.push(name);
		} else if let Some(name) = a.strip_prefix('-') {
			flag_args.push(name);
		} else {
			positional.push(a.clone());
		}
	}
	for f in &flag_args {
		if *f != "json" {
			return Err(usage(format!("Unknown flag: --{f}")));
		}
	}

	if positional.is_empty() {
		return Err(usage("missing unitpm.yml file path".to_string()));
	}
	let path = Path::new(&positional[0]);

	let file = fs::File::open(path).map_err(|e| format!("failed to open unitpm.yml file: {e}"))?;
	let manifest_file = manifest::parse(file).map_err(apply_err)?;
	let specs = manifest_file
		.to_app_specs()
		.map_err(apply_err)?
		.into_iter()
		.map(mutate_spec)
		.collect::<Vec<_>>();

	// Connect only after local validation succeeds.
	if client.is_none() {
		let c = Client::new()?;
		client = Some(Box::new(RealIpc(c)));
	}
	let client = client
		.as_mut()
		.expect("client either provided or opened above");

	let mut rep = Report::new("apply");
	let mut failed_early: Option<Box<dyn std::error::Error>> = None;

	for mut s in specs {
		let id = match spec::generate_id() {
			id if !id.is_empty() => id,
			_ => {
				let err = "failed to generate ID";
				rep.fail(&s.name, Some(&err));
				failed_early = Some(err.into());
				break;
			}
		};
		s.id = id.clone();
		if s.namespace.as_deref().unwrap_or("").is_empty() {
			s.namespace = Some(types::DEFAULT_NAMESPACE.to_string());
		}
		if s.created_at.is_none() {
			s.created_at = Some(now_rfc3339());
		}
		if s.exec.command.is_some() && s.exec.args.is_none() {
			s.exec.args = Some(Vec::new());
		}
		// The Go side populates env if nil; mirror that.
		if s.exec.command.is_none() && s.exec.entry.is_none() {
			let err = "spec must specify command or entry";
			rep.fail(&s.name, Some(&err));
			failed_early = Some(err.into());
			break;
		}

		if let Err(e) = spec::save_spec_protocol(&id, &s) {
			let err_str = e.to_string();
			let full = format!("failed to save spec: {e}");
			rep.fail(&target_label(&s), Some(&full));
			failed_early = Some(err_str.into());
			break;
		}

		let req = StartRequest {
			protocol_version: 1,
			kind: "start".into(),
			request_id: id.clone(),
			spec: s.clone(),
		};

		match client.start(&req) {
			Ok(resp) => {
				let pid = resp.data.as_ref().and_then(|d| d.pid).unwrap_or(0);
				let mut extra = std::collections::BTreeMap::new();
				extra.insert("id".into(), json!(id));
				if pid != 0 {
					extra.insert("pid".into(), json!(pid));
				}
				rep.ok(&target_label(&s), extra);
				if !json_out {
					let target = target_label(&s);
					let _ = writeln!(
						w,
						"{} Applied {}",
						term::green(format_args!("{}", "✓")),
						target
					);
				}
			}
			Err(e) => {
				let full = format!("apply failed for {}: {e}", s.name);
				let target = target_label(&s);
				if !json_out {
					let _ = writeln!(
						w,
						"{} Failed to apply {}: {}",
						term::red(format_args!("{}", "✗")),
						target,
						e
					);
				}
				rep.fail(&target, Some(&full));
				failed_early = Some(Box::new(std::io::Error::other(full)));
				break;
			}
		}
	}

	if json_out {
		rep.emit_json_to(w).map_err(json_err)?;
	} else if rep.summary.total > 1 {
		rep.print_summary(w).map_err(json_err)?;
	}

	if let Some(e) = failed_early {
		return Err(e);
	}
	rep.err().map_or(Ok(()), |e| Err(Box::new(e)))
}

fn apply_err(e: manifest::ManifestError) -> Box<dyn std::error::Error> {
	Box::new(e)
}

fn json_err(e: std::io::Error) -> Box<dyn std::error::Error> {
	Box::new(e)
}

fn usage(msg: String) -> Box<dyn std::error::Error> {
	Box::new(UsageError::new(msg))
}

fn mutate_spec(mut s: AppSpec) -> AppSpec {
	if s.env.is_none() {
		s.env = Some(Default::default());
	}
	s
}

fn target_label(s: &AppSpec) -> String {
	let ns = s
		.namespace
		.as_deref()
		.filter(|n| !n.is_empty())
		.unwrap_or(types::DEFAULT_NAMESPACE);
	format!("{ns}/{}", s.name)
}

fn now_rfc3339() -> String {
	// Minimal RFC3339 formatter using only std. Avoids pulling chrono in.
	use std::time::{SystemTime, UNIX_EPOCH};
	let secs = SystemTime::now()
		.duration_since(UNIX_EPOCH)
		.map(|d| d.as_secs() as i64)
		.unwrap_or(0);
	format_rfc3339_utc(secs)
}

fn format_rfc3339_utc(secs: i64) -> String {
	// Convert epoch seconds to a UTC Y-M-D H:M:S string. Days-since-epoch
	// math keeps the format predictable without depending on the system
	// timezone.
	let secs_per_day = 86_400;
	let days = secs.div_euclid(secs_per_day);
	let secs_in_day = secs.rem_euclid(secs_per_day) as u32;
	let (year, month, day) = civil_from_days(days);
	let hour = secs_in_day / 3600;
	let minute = (secs_in_day / 60) % 60;
	let second = secs_in_day % 60;
	format!(
		"{year:04}-{:02}-{:02}T{:02}:{:02}:{:02}Z",
		month, day, hour, minute, second
	)
}

fn civil_from_days(days_since_epoch: i64) -> (i32, u32, u32) {
	// Howard Hinnant's date conversion. days_since_epoch is days since 1970-01-01.
	let z = days_since_epoch + 719_468;
	let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
	let doe = (z - era * 146_097) as u64;
	let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146_096) / 365;
	let y = yoe as i64 + era * 400;
	let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	let mp = (5 * doy + 2) / 153;
	let d = (doy - (153 * mp + 2) / 5 + 1) as u32;
	let m = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
	let y = if m <= 2 { y + 1 } else { y };
	(y as i32, m, d)
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "apply".to_string(),
		aliases: Vec::new(),
		usage: "unitpm apply <unitpm.yml> [--json]".to_string(),
		description: "Apply a unitpm.yml declarative configuration".to_string(),
		options: vec![
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
			"unitpm apply unitpm.yml".to_string(),
			"unitpm apply config/production.yml".to_string(),
			"unitpm apply unitpm.yml --json | jq '.results'".to_string(),
		],
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
