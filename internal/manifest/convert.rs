//! Conversion from manifest [`AppConfig`] to the protocol's [`AppSpec`].
//!
//! Each manifest app expands to one or more `AppSpec` entries: a single app
//! with `instances > 1` produces that many identical specs, all sharing the
//! same name and namespace. The protocol's `AppSpec.id` is not populated
//! here — that field is assigned when the spec is persisted in phase 6.
//!
//! Logs and restart sections are emitted only when the manifest declares at
//! least one of their fields, matching the Go `buildLogs` / `buildRestart`
//! behaviour of returning `nil` for an all-empty struct.

use crate::ipc::protocol::{AppExec, AppLogs, AppRestart, AppSpec};
use crate::manifest::tokenize::tokenize_command;
use crate::manifest::{AppConfig, File, ManifestError};
use crate::types;

/// Extension trait that mirrors the Go `File.ToAppSpecs` method.
pub trait ToAppSpecs {
	fn to_app_specs(&self) -> Result<Vec<AppSpec>, ManifestError>;
}

impl ToAppSpecs for File {
	fn to_app_specs(&self) -> Result<Vec<AppSpec>, ManifestError> {
		let mut specs = Vec::new();
		for app in &self.apps {
			let base = to_app_spec(app, &self.namespace)?;
			let instances = if app.instances < 1 { 1 } else { app.instances };
			for _ in 0..instances {
				specs.push(base.clone());
			}
		}
		Ok(specs)
	}
}

/// Convert a single manifest app into the protocol's spec shape, applying the
/// fallback chain `app.namespace → file.namespace → "default"`.
pub fn to_app_spec(app: &AppConfig, default_namespace: &str) -> Result<AppSpec, ManifestError> {
	let ns = if !app.namespace.is_empty() {
		app.namespace.clone()
	} else if !default_namespace.is_empty() {
		default_namespace.to_string()
	} else {
		types::DEFAULT_NAMESPACE.to_string()
	};

	let exec = if !app.command.is_empty() {
		let cmd_parts = tokenize_command(&app.command)
			.map_err(|_| ManifestError::InvalidCommand(app.name.clone()))?;
		if cmd_parts.is_empty() {
			return Err(ManifestError::InvalidCommand(app.name.clone()));
		}
		AppExec {
			kind: "command".to_string(),
			command: Some(cmd_parts[0].clone()),
			args: Some(cmd_parts[1..].to_vec()),
			entry: None,
			runtime: None,
			shell: false,
		}
	} else {
		AppExec {
			kind: "entry".to_string(),
			command: None,
			args: None,
			entry: Some(app.entry.clone()),
			runtime: if app.runtime.is_empty() {
				None
			} else {
				Some(app.runtime.clone())
			},
			shell: false,
		}
	};

	Ok(AppSpec {
		version: 1,
		id: String::new(),
		name: app.name.clone(),
		namespace: Some(ns),
		exec,
		cwd: if app.cwd.is_empty() {
			None
		} else {
			Some(app.cwd.clone())
		},
		env: if app.env.is_empty() {
			None
		} else {
			Some(app.env.clone())
		},
		env_file: None,
		logs: build_logs(&app.logs).map(Box::new),
		restart: build_restart(app).map(Box::new),
		cron: None,
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	})
}

fn build_logs(logs: &crate::manifest::LogsConfig) -> Option<AppLogs> {
	if !logs.dir.is_empty()
		|| !logs.stdout.is_empty()
		|| !logs.stderr.is_empty()
		|| !logs.format.is_empty()
		|| !logs.timestamp.is_empty()
	{
		Some(AppLogs {
			mode: String::new(),
			dir: if logs.dir.is_empty() {
				None
			} else {
				Some(logs.dir.clone())
			},
			stdout: if logs.stdout.is_empty() {
				None
			} else {
				Some(logs.stdout.clone())
			},
			stderr: if logs.stderr.is_empty() {
				None
			} else {
				Some(logs.stderr.clone())
			},
			format: if logs.format.is_empty() {
				None
			} else {
				Some(logs.format.clone())
			},
			timestamp: if logs.timestamp.is_empty() {
				None
			} else {
				Some(logs.timestamp.clone())
			},
		})
	} else {
		None
	}
}

fn build_restart(app: &AppConfig) -> Option<AppRestart> {
	if !app.restart.policy.is_empty() {
		Some(AppRestart {
			policy: app.restart.policy.clone(),
			max_retries: if app.restart.max_restarts == 0 {
				None
			} else {
				Some(app.restart.max_restarts)
			},
			backoff_ms: if app.restart.delay_ms == 0 {
				None
			} else {
				Some(app.restart.delay_ms)
			},
			backoff_type: if app.restart.backoff.is_empty() {
				None
			} else {
				Some(app.restart.backoff.clone())
			},
			stop_on_exit: if app.restart.stop_on_exit.is_empty() {
				None
			} else {
				Some(app.restart.stop_on_exit.clone())
			},
		})
	} else {
		None
	}
}
