//! The `version` command.
//!
//! 10 cases ported from `internal/cli/commands/version/cmd_test.go`.
//!
//! Reports the local CLI build info and, when reachable, the daemon's
//! matching build info. JSON mode is supported via `--json`; non-JSON
//! mode renders a human block. Quiet mode suppresses the human block but
//! still emits JSON on request.

use std::io::Write;

use serde::Serialize;

use crate::cli::help::CommandSpec;
use crate::ipc::protocol::MismatchData;
use crate::ipc::transport::{Client, IPCClient, TransportError};
use crate::term;
use crate::version::{self, Info};

/// Dyn-compatible subset of the IPC surface this command depends on.
/// Each command owns its own trait like this so the dispatcher and tests
/// can hold a `Box<dyn Ipc>` without inheriting the generic-method
/// problem of [`crate::ipc::transport::IPCClient`].
pub trait Ipc {
	/// Returns the daemon's `version` build info. Mirrors the
	/// `version_handler` verb on the server side.
	fn version(&mut self) -> Result<Info, TransportError>;
}

/// Production adapter that wraps the real [`Client`].
pub struct RealIpc(pub Client);

impl Ipc for RealIpc {
	fn version(&mut self) -> Result<Info, TransportError> {
		let mut info = Info {
			version: String::new(),
			commit: String::new(),
			build_date: String::new(),
			protocol_version: 0,
		};
		self.0.call::<(), _>("version", None, Some(&mut info))?;
		Ok(info)
	}
}

/// Run the version command. `client` may be `None`; when it is, a default
/// client is opened. `w` receives the rendered block; tests pass a sink.
pub fn run<W: Write>(
	mut client: Option<Box<dyn Ipc>>,
	w: &mut W,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	let json_output = args.iter().any(|a| a == "--json");

	for a in args {
		if !a.starts_with('-') {
			return Err(unexpected_args(std::slice::from_ref(a)));
		}
	}

	let local = version::get();

	let mut daemon_info: Option<Info> = None;
	let mut daemon_err: Option<TransportError> = None;

	if client.is_none() {
		match Client::new() {
			Ok(c) => client = Some(Box::new(RealIpc(c))),
			Err(e) => daemon_err = Some(e),
		}
	}

	if let Some(c) = client.as_mut() {
		match c.version() {
			Ok(info) => daemon_info = Some(info),
			Err(e) => daemon_err = Some(e),
		}
	}

	if json_output {
		emit_json(w, &local, daemon_info.as_ref())?;
		return Ok(());
	}

	if term::is_quiet() {
		return Ok(());
	}

	writeln!(w)?;
	writeln!(w, "{}", term::cyan(format_args!("{}", "unitpm CLI")))?;
	print_info(w, &local);

	match (daemon_info.as_ref(), daemon_err.as_ref()) {
		(Some(di), _) => {
			writeln!(w)?;
			writeln!(w, "{}", term::cyan(format_args!("{}", "unitpmd daemon")))?;
			print_info(w, di);

			writeln!(w)?;
			writeln!(w, "{}", term::cyan(format_args!("{}", "Protocol")))?;
			writeln!(
				w,
				"  {} : {}",
				term::dim(format_args!("{}", "CLI")),
				term::bold(format_args!("v{}", local.protocol_version))
			)?;
			writeln!(
				w,
				"  {} : {}",
				term::dim(format_args!("{}", "Daemon")),
				term::bold(format_args!("v{}", di.protocol_version))
			)?;
		}
		(None, Some(err)) => {
			if !handle_protocol_mismatch(w, &local, err) {
				writeln!(w)?;
				writeln!(w, "{}", term::cyan(format_args!("{}", "Protocol")))?;
				writeln!(
					w,
					"  {} : {}",
					term::dim(format_args!("{}", "CLI")),
					term::bold(format_args!("v{}", local.protocol_version))
				)?;
			}
		}
		(None, None) => {
			writeln!(w)?;
			writeln!(w, "{}", term::cyan(format_args!("{}", "Protocol")))?;
			writeln!(
				w,
				"  {} : {}",
				term::dim(format_args!("{}", "CLI")),
				term::bold(format_args!("v{}", local.protocol_version))
			)?;
		}
	}

	Ok(())
}

fn unexpected_args(args: &[String]) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(format!(
		"Unexpected arguments: {}",
		args.iter()
			.map(|s| format!("\"{s}\""))
			.collect::<Vec<_>>()
			.join(" ")
	))
}

#[derive(Serialize)]
struct VersionEntry<'a> {
	version: &'a str,
	commit: &'a str,
	build_date: &'a str,
}

#[derive(Serialize)]
struct ProtocolEntry {
	cli: i64,
	daemon: Option<i64>,
}

#[derive(Serialize)]
struct JsonOutput<'a> {
	cli: VersionEntry<'a>,
	daemon: Option<VersionEntry<'a>>,
	protocol: ProtocolEntry,
}

fn emit_json<W: Write>(
	w: &mut W,
	local: &Info,
	daemon: Option<&Info>,
) -> Result<(), Box<dyn std::error::Error>> {
	let cli = VersionEntry {
		version: &local.version,
		commit: &local.commit,
		build_date: &local.build_date,
	};
	let daemon_entry = daemon.map(|d| VersionEntry {
		version: &d.version,
		commit: &d.commit,
		build_date: &d.build_date,
	});
	let protocol = ProtocolEntry {
		cli: local.protocol_version,
		daemon: daemon.map(|d| d.protocol_version),
	};
	let out = JsonOutput {
		cli,
		daemon: daemon_entry,
		protocol,
	};
	let bytes = crate::jsonx::marshal(&out)?;
	w.write_all(&bytes)?;
	writeln!(w)?;
	Ok(())
}

fn print_info<W: Write>(w: &mut W, info: &Info) {
	writeln!(
		w,
		"  {} : {}",
		term::dim(format_args!("{}", "Version")),
		term::bold(format_args!("{}", info.version))
	)
	.ok();
	writeln!(
		w,
		"  {} : {}",
		term::dim(format_args!("{}", "Commit")),
		term::bold(format_args!("{}", info.commit))
	)
	.ok();
	writeln!(
		w,
		"  {} : {}",
		term::dim(format_args!("{}", "Built")),
		term::bold(format_args!("{}", info.build_date))
	)
	.ok();
}

fn handle_protocol_mismatch<W: Write>(w: &mut W, local: &Info, err: &TransportError) -> bool {
	let remote = match err {
		TransportError::Remote(r) => r,
		_ => return false,
	};
	if remote.code != "PROTOCOL_MISMATCH" {
		return false;
	}

	let supported = remote
		.data
		.as_ref()
		.and_then(|v| serde_json::from_value::<MismatchData>(v.clone()).ok())
		.map(|d| d.supported)
		.unwrap_or(0);

	writeln!(w).ok();
	writeln!(w, "{}", term::cyan(format_args!("{}", "Protocol"))).ok();
	writeln!(
		w,
		"  {} : {}",
		term::dim(format_args!("{}", "CLI")),
		term::bold(format_args!("v{}", local.protocol_version))
	)
	.ok();
	if supported > 0 {
		writeln!(
			w,
			"  {} : {}",
			term::dim(format_args!("{}", "Daemon")),
			term::bold(format_args!("v{}", supported))
		)
		.ok();
	} else {
		writeln!(
			w,
			"  {} : {}",
			term::dim(format_args!("{}", "Daemon")),
			term::bold(format_args!("{}", "unknown"))
		)
		.ok();
	}

	writeln!(w).ok();
	writeln!(
		w,
		"{}",
		term::red(format_args!("{}", "Error: Protocol mismatch"))
	)
	.ok();
	if supported > 0 {
		writeln!(
			w,
			"The CLI (v{}) and Daemon (v{}) have incompatible protocols.",
			local.protocol_version, supported
		)
		.ok();
	} else {
		writeln!(
			w,
			"The CLI (v{}) and Daemon have incompatible protocols.",
			local.protocol_version
		)
		.ok();
	}

	true
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let spec = spec();
	let _ = crate::cli::help::render_command_help(w, &spec);
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "version".to_string(),
		aliases: Vec::new(),
		usage: "unitpm version".to_string(),
		description: "Show version information for CLI and Daemon.".to_string(),
		options: vec![
			crate::cli::help::Option {
				short: String::new(),
				long: "--json".to_string(),
				description: "Output version info as JSON.".to_string(),
			},
			crate::cli::help::Option {
				short: "-h".to_string(),
				long: "--help".to_string(),
				description: "Show this help message.".to_string(),
			},
		],
		examples: Vec::new(),
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
