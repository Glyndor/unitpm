//! The `export` command.
//!
//! 10 cases ported from `internal/cli/commands/export/cmd_test.go`.
//!
//! Renders the on-disk specs in a single namespace as a unitpm.yml
//! manifest. The `--namespace <name>` / `-n <name>` selector is required.
//! The output is YAML on stdout; the CLI does not call the daemon.

use std::collections::BTreeMap;
use std::io::Write;

use crate::cli::errs::UsageError;
use crate::cli::help::CommandSpec;
use crate::ipc::protocol::{AppExec, AppLogs, AppRestart, AppSpec};
use crate::spec;
use crate::types;

/// Run the `export` command.
pub fn run<W: Write>(w: &mut W, args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	if args.len() < 2 {
		return Err(usage("export requires --namespace <name>".to_string()));
	}

	let mut namespace = String::new();
	let mut i = 0;
	while i < args.len() {
		let arg = &args[i];
		if arg == "--namespace" || arg == "-n" {
			if i + 1 >= args.len() {
				return Err(usage("missing value for --namespace".to_string()));
			}
			namespace = args[i + 1].clone();
			i += 2;
			continue;
		}
		if arg.starts_with("--namespace=") {
			namespace = arg.trim_start_matches("--namespace=").to_string();
		} else if arg.starts_with("-n=") {
			namespace = arg.trim_start_matches("-n=").to_string();
		}
		i += 1;
	}

	if namespace.is_empty() {
		return Err(usage("missing --namespace".to_string()));
	}

	let specs = spec::load_all_protocol().map_err(spec_err)?;

	let mut file = YamlFile {
		version: "1".to_string(),
		namespace: namespace.clone(),
		apps: Vec::new(),
	};

	for s in specs {
		let ns = if s.namespace.as_deref().unwrap_or("").is_empty() {
			types::DEFAULT_NAMESPACE.to_string()
		} else {
			s.namespace.clone().unwrap()
		};
		if ns != namespace {
			continue;
		}

		let mut app = YamlApp {
			name: s.name.clone(),
			command: String::new(),
			entry: String::new(),
			runtime: String::new(),
			cwd: s.cwd.clone().unwrap_or_default(),
			env: s.env.clone().unwrap_or_default(),
			logs: None,
			restart: None,
		};

		match s.exec.kind.as_str() {
			"command" => {
				let cmd = s.exec.command.clone().unwrap_or_default();
				let args = s.exec.args.clone().unwrap_or_default();
				if !args.is_empty() {
					app.command = format!("{} {}", cmd, args.join(" "));
				} else {
					app.command = cmd;
				}
			}
			"entry" => {
				app.entry = s.exec.entry.clone().unwrap_or_default();
				app.runtime = s.exec.runtime.clone().unwrap_or_default();
			}
			_ => {}
		}

		if let Some(logs) = &s.logs {
			app.logs = Some(YamlLogs {
				dir: logs.dir.clone().unwrap_or_default(),
				stdout: logs.stdout.clone().unwrap_or_default(),
				stderr: logs.stderr.clone().unwrap_or_default(),
				format: logs.format.clone().unwrap_or_default(),
				timestamp: logs.timestamp.clone().unwrap_or_default(),
			});
		}
		if let Some(restart) = &s.restart {
			app.restart = Some(YamlRestart {
				policy: restart.policy.clone(),
				max_restarts: restart.max_retries.unwrap_or(0),
				delay_ms: restart.backoff_ms.unwrap_or(0),
				backoff: restart.backoff_type.clone().unwrap_or_default(),
				stop_on_exit: restart.stop_on_exit.clone().unwrap_or_default(),
			});
		}

		file.apps.push(app);
	}

	if file.apps.is_empty() {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"no apps found in namespace {:?}",
			namespace
		)));
	}

	let bytes = serde_yaml::to_string(&file).map_err(yaml_err)?;
	w.write_all(bytes.as_bytes())?;
	Ok(())
}

fn usage(msg: String) -> Box<dyn std::error::Error> {
	Box::new(UsageError::new(msg))
}

fn spec_err(e: crate::spec::SpecError) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(format!("failed to load specs: {e}"))
}

fn yaml_err(e: serde_yaml::Error) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(format!("failed to encode unitpm.yml file: {e}"))
}

/// Internal YAML shape mirroring the Go `unitpm.yml file.File` / `AppConfig`.
/// Kept private — the on-disk format is consumed by `manifest::File`
/// when `apply` reads it back.
#[derive(serde::Serialize)]
struct YamlFile {
	version: String,
	namespace: String,
	apps: Vec<YamlApp>,
}

#[derive(serde::Serialize)]
struct YamlApp {
	name: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	command: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	entry: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	runtime: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	cwd: String,
	#[serde(skip_serializing_if = "BTreeMap::is_empty")]
	env: BTreeMap<String, String>,
	#[serde(skip_serializing_if = "Option::is_none")]
	logs: Option<YamlLogs>,
	#[serde(skip_serializing_if = "Option::is_none")]
	restart: Option<YamlRestart>,
}

#[derive(serde::Serialize)]
struct YamlLogs {
	#[serde(skip_serializing_if = "String::is_empty")]
	dir: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	stdout: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	stderr: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	format: String,
	#[serde(skip_serializing_if = "String::is_empty")]
	timestamp: String,
}

#[derive(serde::Serialize)]
struct YamlRestart {
	policy: String,
	#[serde(rename = "max_restarts")]
	max_restarts: i32,
	#[serde(rename = "delay_ms")]
	delay_ms: i32,
	backoff: String,
	#[serde(rename = "stop_on_exit")]
	stop_on_exit: Vec<i32>,
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "export".to_string(),
		aliases: Vec::new(),
		usage: "unitpm export --namespace <name>".to_string(),
		description: "Export current applications in a namespace to unitpm.yml format"
			.to_string(),
		options: vec![crate::cli::help::Option {
			short: "-h".to_string(),
			long: "--help".to_string(),
			description: "Show this help message.".to_string(),
		}],
		examples: Vec::new(),
		hidden: false,
	}
}

#[allow(dead_code)]
fn _phantom_app_exec() -> Option<AppExec> {
	None
}
#[allow(dead_code)]
fn _phantom_app_logs() -> Option<AppLogs> {
	None
}
#[allow(dead_code)]
fn _phantom_app_restart() -> Option<AppRestart> {
	None
}
#[allow(dead_code)]
fn _phantom_app_spec() -> Option<AppSpec> {
	None
}

#[cfg(test)]
mod tests;
