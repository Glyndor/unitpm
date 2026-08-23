//! Dry-run output for the `start` command.
//!
//! `--dry-run` asks the daemon to do nothing — parse, validate, and
//! print the resulting [`AppSpec`] as a key/value table so the
//! operator can confirm what would have been spawned before
//! committing. `--json` flips the table for `{"spec": ..., "scale":
//! ...}`.
//!
//! The Go side renders the dry-run output inline in the command; we
//! keep the same shape here so the textual comparison fixtures still
//! line up.

use std::io::{self, Write};
use std::result::Result;

use crate::cli::table::{self, KvRow};
use crate::term;

pub(crate) fn print_dry_run(
	out: &mut impl Write,
	spec: &crate::ipc::protocol::AppSpec,
	scale: i32,
	json_out: bool,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if json_out {
		let shape = serde_json::json!({ "spec": spec, "scale": scale });
		let bytes = serde_json::to_vec(&shape)
			.map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })?;
		out.write_all(&bytes)?;
		writeln!(out)?;
		return Ok(());
	}

	let _ = writeln!(
		out,
		"{} Dry run — would start {} instance(s)\n",
		term::cyan(format_args!("{}", "i")),
		scale
	);

	let mut rows: Vec<KvRow> = Vec::new();
	rows.push([
		"command".to_string(),
		spec.exec.command.clone().unwrap_or_default(),
	]);
	if let Some(args) = &spec.exec.args {
		if !args.is_empty() {
			rows.push(["args".to_string(), args.join(" ")]);
		}
	}
	if !spec.exec.entry.as_deref().unwrap_or("").is_empty() {
		let entry = spec.exec.entry.clone().unwrap_or_default();
		let runtime = spec.exec.runtime.clone().unwrap_or_default();
		rows.push(["entry".to_string(), format!("{entry} ({runtime})")]);
	}
	rows.push(["cwd".to_string(), spec.cwd.clone().unwrap_or_default()]);
	rows.push([
		"namespace".to_string(),
		spec.namespace.clone().unwrap_or_default(),
	]);
	rows.push(["name".to_string(), spec.name.clone()]);
	if let Some(run_as) = &spec.run_as {
		if run_as.mode != "self" {
			rows.push(["isolation".to_string(), run_as.mode.clone()]);
		}
	}
	if let Some(cron) = &spec.cron {
		rows.push(["schedule".to_string(), cron.clone()]);
	}
	if let Some(restart) = &spec.restart {
		rows.push([
			"restart".to_string(),
			format!(
				"policy={} max={} backoff={}",
				restart.policy,
				restart.max_retries.unwrap_or(0),
				restart.backoff_type.clone().unwrap_or_default()
			),
		]);
	}
	if let Some(env_file) = &spec.env_file {
		rows.push(["env-file".to_string(), env_file.clone()]);
	}

	let stdout = io::stdout();
	let mut lock = stdout.lock();
	let _ = table::kv(&mut lock, "Spec", &rows);
	// Note: the Go code prints to stdout directly; tests catch it via
	// `captureStdout`. Out-of-scope for unit tests.
	Ok(())
}
