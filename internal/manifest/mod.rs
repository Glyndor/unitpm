//! Declarative manifest parser.
//!
//! Reads the YAML manifest file (`unitpm.yml` at the project root) and decodes
//! it into the daemon's `AppSpec` shape. The pre-org manifest name was never
//! released, so no migration path is needed — there is no release, no user has
//! the old file on disk, and the parser only looks for the new name.
//!
//! Phase 3 of #51. The Go counterpart stays in place until phase 7 deletes
//! the old code.

mod convert;
mod tokenize;

use std::collections::BTreeMap;
use std::io::Read;

use serde::Deserialize;

/// Top-level manifest shape. Mirrors the Go `File` in the legacy manifest package.
#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct File {
	#[serde(default)]
	pub version: String,
	#[serde(default)]
	pub namespace: String,
	#[serde(default)]
	pub apps: Vec<AppConfig>,
}

/// Configuration for a single application within a manifest.
#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct AppConfig {
	#[serde(default)]
	pub name: String,
	#[serde(default)]
	pub namespace: String,
	#[serde(default)]
	pub command: String,
	#[serde(default)]
	pub entry: String,
	#[serde(default)]
	pub runtime: String,
	#[serde(default)]
	pub cwd: String,
	#[serde(default)]
	pub env: BTreeMap<String, String>,
	#[serde(default)]
	pub instances: i32,
	#[serde(default)]
	pub restart: RestartConfig,
	#[serde(default)]
	pub logs: LogsConfig,
}

/// Restart policy and backoff parameters.
#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
pub struct RestartConfig {
	#[serde(default)]
	pub policy: String,
	#[serde(default, rename = "max_restarts")]
	pub max_restarts: i32,
	#[serde(default, rename = "delay_ms")]
	pub delay_ms: i32,
	#[serde(default)]
	pub backoff: String,
	#[serde(default, rename = "stop_on_exit")]
	pub stop_on_exit: Vec<i32>,
}

/// Logging configuration for an application.
#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
pub struct LogsConfig {
	#[serde(default)]
	pub dir: String,
	#[serde(default)]
	pub stdout: String,
	#[serde(default)]
	pub stderr: String,
	#[serde(default)]
	pub format: String,
	#[serde(default)]
	pub timestamp: String,
}

/// Errors surfaced by the manifest parser.
#[derive(Debug)]
pub enum ManifestError {
	Io(std::io::Error),
	Yaml(serde_yaml::Error),
	NoApps,
	EmptyAppName,
	BothCommandAndEntry(String),
	NeitherCommandNorEntry(String),
	InvalidCommand(String),
}

impl std::fmt::Display for ManifestError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			ManifestError::Io(e) => write!(f, "failed to read manifest: {e}"),
			ManifestError::Yaml(e) => write!(f, "failed to parse manifest: {e}"),
			ManifestError::NoApps => f.write_str("manifest has no apps"),
			ManifestError::EmptyAppName => f.write_str("manifest app has empty name"),
			ManifestError::BothCommandAndEntry(name) => {
				write!(f, "manifest app {name} has both command and entry")
			}
			ManifestError::NeitherCommandNorEntry(name) => {
				write!(f, "manifest app {name} must specify command or entry")
			}
			ManifestError::InvalidCommand(name) => {
				write!(f, "invalid command for app {name}")
			}
		}
	}
}

impl std::error::Error for ManifestError {}

impl From<std::io::Error> for ManifestError {
	fn from(e: std::io::Error) -> Self {
		ManifestError::Io(e)
	}
}

/// Read and decode a manifest from any byte source. Validates that at least
/// one app exists and that each app's name and execution shape are well-formed.
pub fn parse<R: Read>(mut reader: R) -> Result<File, ManifestError> {
	let mut buf = Vec::new();
	reader.read_to_end(&mut buf)?;
	let file: File = serde_yaml::from_slice(&buf).map_err(ManifestError::Yaml)?;
	validate(&file)?;
	Ok(file)
}

fn validate(file: &File) -> Result<(), ManifestError> {
	if file.apps.is_empty() {
		return Err(ManifestError::NoApps);
	}
	for app in &file.apps {
		if app.name.trim().is_empty() {
			return Err(ManifestError::EmptyAppName);
		}
		let has_command = !app.command.is_empty();
		let has_entry = !app.entry.is_empty();
		if has_command && has_entry {
			return Err(ManifestError::BothCommandAndEntry(app.name.clone()));
		}
		if !has_command && !has_entry {
			return Err(ManifestError::NeitherCommandNorEntry(app.name.clone()));
		}
	}
	Ok(())
}

pub use convert::{to_app_spec, ToAppSpecs};

#[cfg(all(test, target_os = "linux"))]
mod tests;
