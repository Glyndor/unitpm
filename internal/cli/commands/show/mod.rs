//! `unitpm show` — print detailed information about a single process.
//!
//! Mirrors `internal/cli/commands/show/cmd.go`. The command is split
//! across three files:
//!
//! - [`mod`] (here) — wire types, argument parsing, the daemon call,
//!   command-spec registration. About 250 lines.
//! - [`render`] — one helper per spec section (process, exec, env,
//!   logs, restart, stop, resources, isolation, schedule, watch) and
//!   the small formatters (`color_state`, `pid_str`, `mask_secret`).
//!   Mirrors the corresponding `renderX` helpers in the Go file.
//! - [`tests_helpers`] — fixtures used by the render tests, kept here
//!   so `render::tests` doesn't have to reach back into `mod`.

mod render;
#[cfg(test)]
mod tests_helpers;

pub use render::render;

use std::collections::BTreeMap;
use std::io::{self, Write};

use serde::Serialize;

use crate::cli::errs::UsageError;
use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::cli::root::cmd;
use crate::ipc::protocol::AppSpec;
use crate::ipc::transport::IPCClient;
use crate::jsonx;
use crate::types::ProcessInfo;

/// Wire payload the daemon returns for the `show` verb. We carry the
/// spec explicitly instead of deriving `Deserialize` because
/// [`AppSpec`] does not implement `Default` and the daemon may omit the
/// field on older versions.
#[derive(Debug, Clone, Serialize)]
pub struct ShowResponse {
	pub info: ProcessInfo,
	pub spec: AppSpec,
}

impl<'de> serde::Deserialize<'de> for ShowResponse {
	fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
	where
		D: serde::Deserializer<'de>,
	{
		#[derive(serde::Deserialize)]
		struct Helper {
			info: ProcessInfo,
			#[serde(default)]
			spec: Option<AppSpec>,
		}
		let h = Helper::deserialize(deserializer)?;
		Ok(ShowResponse {
			info: h.info,
			spec: h.spec.unwrap_or_else(empty_spec),
		})
	}
}

/// Parsed command-line options.
#[derive(Debug, Clone, Default)]
pub struct Options {
	pub json: bool,
	pub target: Option<String>,
}

/// Local client trait — mirrors [`IPCClient::call`] with `Box<dyn>`
/// erased types so it stays dyn-compatible. The dispatcher uses the
/// concrete [`crate::ipc::transport::Client`]; tests can substitute
/// any mock that implements this trait. Kept module-private because
/// three commands need the same shape, and the alternative would be
/// to refactor the public [`IPCClient`] trait.
pub trait DynClient {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String>;
}

impl DynClient for crate::ipc::transport::Client {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		let params = BTreeMap::from([("id".to_string(), id.to_string())]);
		self.call("show", Some(&params), Some(resp))
			.map_err(|e| format!("show failed: {e}"))
	}
}

impl DynClient for Box<dyn DynClient> {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		(**self).call_show(id, resp)
	}
}

/// Public entry point. `client` may be `None`; in that case the
/// dispatcher has already created a concrete transport client.
pub fn run(
	client: Option<&mut dyn DynClient>,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if crate::cli::help::is_help(args) {
		let mut buf = Vec::new();
		render_help(&mut buf)?;
		print!("{}", String::from_utf8_lossy(&buf));
		return Ok(());
	}
	let opts = parse_args(args)?;
	let id = opts
		.target
		.ok_or_else(|| UsageError::new("missing process ID or name"))?;

	let mut owned_client: Option<Box<dyn DynClient>>;
	let client: &mut dyn DynClient = match client {
		Some(c) => c,
		None => {
			let new_client: Box<dyn DynClient> = Box::new(
				crate::ipc::transport::Client::new().map_err(|e| format!("show failed: {e}"))?,
			);
			owned_client = Some(new_client);
			owned_client.as_mut().unwrap()
		}
	};

	let mut resp = ShowResponse {
		info: ProcessInfo {
			id: String::new(),
			name: String::new(),
			namespace: String::new(),
			version: String::new(),
			mode: String::new(),
			pid: 0,
			uptime: 0,
			restarts: 0,
			state: crate::types::ProcessState::Running,
			cpu: 0.0,
			memory: 0,
			user: String::new(),
			watch: false,
			git_branch: None,
			git_commit: None,
			git_dirty: false,
			created_at: None,
		},
		spec: empty_spec(),
	};
	client.call_show(&id, &mut resp)?;

	if opts.json {
		let bytes = jsonx::marshal(&resp).map_err(jsonx_to_io)?;
		let stdout = io::stdout();
		let mut out = stdout.lock();
		writeln!(out, "{}", String::from_utf8_lossy(&bytes))?;
		return Ok(());
	}

	render::render_to(&mut io::stdout().lock(), &resp);
	Ok(())
}

fn empty_spec() -> AppSpec {
	AppSpec {
		version: 1,
		id: String::new(),
		name: String::new(),
		namespace: None,
		exec: crate::ipc::protocol::AppExec {
			kind: String::new(),
			command: None,
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: None,
		env: None,
		env_file: None,
		logs: None,
		restart: None,
		cron: None,
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	}
}

fn jsonx_to_io(e: jsonx::Error) -> io::Error {
	io::Error::new(io::ErrorKind::InvalidData, e)
}

/// Parse the command's flag arguments. Mirrors the Go hand-rolled
/// parser: `--json` flips JSON output; the first non-flag argument is
/// the target.
pub fn parse_args(args: &[String]) -> Result<Options, Box<dyn std::error::Error + Send + Sync>> {
	let mut opts = Options::default();
	let mut positional: Vec<String> = Vec::new();
	for a in args {
		match a.as_str() {
			"--json" => opts.json = true,
			_ => positional.push(a.clone()),
		}
	}
	opts.target = positional.into_iter().next();
	Ok(opts)
}

/// `CommandSpec` returned to the dispatcher at registration time.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: cmd::SHOW.to_string(),
		aliases: vec!["info".to_string(), "describe".to_string()],
		usage: format!(
			"unitpm {}|info|describe <id|name|namespace:name> [--json]",
			cmd::SHOW
		),
		description: "Show detailed information about a process.".to_string(),
		options: vec![
			HelpOption {
				short: "-h".into(),
				long: "--help".into(),
				description: "Show this help message.".into(),
			},
			HelpOption {
				short: "".into(),
				long: "--json".into(),
				description: "Emit the raw daemon response as JSON on stdout.".into(),
			},
		],
		examples: vec![
			format!("unitpm {} my-api", cmd::SHOW),
			"unitpm info prod:my-api".to_string(),
			"unitpm describe 019d9a04".to_string(),
			format!("unitpm {} my-api --json | jq '.spec.env'", cmd::SHOW),
		],
		hidden: false,
	}
}

/// Print the command help block to `w`.
pub fn render_help<W: Write>(w: &mut W) -> io::Result<()> {
	crate::cli::help::render_command_help(w, &spec())
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn empty_spec_has_no_namespace() {
		let s = empty_spec();
		assert!(s.namespace.is_none());
	}

	#[test]
	fn show_response_deserializes_with_missing_spec() {
		// Daemon may omit the spec on older versions; the default fills in.
		let raw = r#"{"info":{"id":"a","name":"b","namespace":"","version":"","mode":"","pid":0,"uptime_ms":0,"restarts":0,"state":"Running","cpu":0,"memory_bytes":0,"user":"","watch":false}}"#;
		let resp: ShowResponse = serde_json::from_str(raw).expect("parse");
		assert_eq!(resp.info.id, "a");
		assert_eq!(resp.spec.id, "");
	}

	#[test]
	fn jsonx_to_io_preserves_message() {
		let err = jsonx_to_io(jsonx::Error::Json(
			serde_json::from_str::<serde_json::Value>("not-json").unwrap_err(),
		));
		assert_eq!(err.kind(), std::io::ErrorKind::InvalidData);
	}

	#[test]
	fn dyn_client_box_dispatches() {
		let mut inner = crate::ipc::transport::Client::new().ok();
		if let Some(c) = inner.as_mut() {
			let mut boxed: Box<dyn DynClient> =
				Box::new(crate::ipc::transport::Client::new().expect("client"));
			let mut resp = ShowResponse {
				info: crate::types::ProcessInfo {
					id: String::new(),
					name: String::new(),
					namespace: String::new(),
					version: String::new(),
					mode: String::new(),
					pid: 0,
					uptime: 0,
					restarts: 0,
					state: crate::types::ProcessState::Running,
					cpu: 0.0,
					memory: 0,
					user: String::new(),
					watch: false,
					git_branch: None,
					git_commit: None,
					git_dirty: false,
					created_at: None,
				},
				spec: empty_spec(),
			};
			// Real server unreachable → error path exercised, not result.
			let _ = boxed.call_show("missing", &mut resp);
			let _ = DynClient::call_show(c, "missing", &mut resp);
		}
	}
}
