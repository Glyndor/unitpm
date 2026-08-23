//! Application specification persistence.
//!
//! Specs live as JSON files under `$XDG_CONFIG_HOME/unitpm/apps/<id>.json`
//! (default `~/.config/unitpm/apps/`). File mode `0600`, directory mode `0700`.
//! One UUID v7 per spec — the ID is also the filename stem.
//!
//! Two flavours of spec live in this crate:
//!
//! - The phase-1 [`AppSpec`] / [`AppExec`] (kept for the three tests that
//!   pinned the directory layout in earlier phases).
//! - The protocol package's full [`protocol::AppSpec`](crate::ipc::protocol::AppSpec)
//!   used by the daemon manager. Saved / loaded via [`save_spec_protocol`]
//!   and [`load_spec_protocol`], and bulk-loaded via [`load_all_protocol`].
//!
//! The two write to the same directory; the local shape is only used by
//! legacy callers and is JSON-distinguishable from the protocol shape.

use std::fs;
use std::path::PathBuf;

use uuid::Uuid;

use crate::ipc::protocol::AppSpec as ProtocolAppSpec;
use crate::jsonx;

/// On-disk application specification. Local to phase 1 — superseded by the
/// protocol package's `AppSpec` for everything except the directory-layout
/// tests.
#[derive(Debug, Clone, PartialEq, serde::Serialize, serde::Deserialize)]
pub struct AppSpec {
	pub name: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub namespace: Option<String>,
	pub exec: AppExec,
}

/// Execution shape carried by [`AppSpec`]. Minimal placeholder until the
/// protocol package lands.
#[derive(Debug, Clone, PartialEq, serde::Serialize, serde::Deserialize)]
pub struct AppExec {
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub command: Option<String>,
}

/// Local default namespace, mirrors `protocol.AppSpec.Namespace` zero-value.
pub const DEFAULT_NAMESPACE: &str = "default";

/// Generate a new UUID v7 spec ID. Time-ordered so on-disk listings track
/// insertion order without sorting.
#[must_use]
pub fn generate_id() -> String {
	Uuid::now_v7().to_string()
}

/// Return the directory where specs are stored, creating it (`0700`) if
/// missing.
pub fn get_spec_dir() -> Result<PathBuf, SpecError> {
	let config_home = match std::env::var("XDG_CONFIG_HOME") {
		Ok(v) if !v.is_empty() => PathBuf::from(v),
		_ => match std::env::var("HOME") {
			Ok(h) if !h.is_empty() => PathBuf::from(h).join(".config"),
			_ => return Err(SpecError::NoHome),
		},
	};
	let dir = config_home.join("unitpm").join("apps");
	fs::create_dir_all(&dir).map_err(SpecError::Io)?;
	#[cfg(unix)]
	{
		use std::os::unix::fs::PermissionsExt;
		let _ = fs::set_permissions(&dir, fs::Permissions::from_mode(0o700));
	}
	Ok(dir)
}

/// Load the local-phase spec with the given ID.
pub fn load_spec(id: &str) -> Result<AppSpec, SpecError> {
	let path = spec_path(id)?;
	let bytes = fs::read(&path).map_err(SpecError::Io)?;
	let mut spec: AppSpec = jsonx::unmarshal(&bytes).map_err(SpecError::Json)?;
	if spec.namespace.as_deref().unwrap_or("").is_empty() {
		spec.namespace = Some(DEFAULT_NAMESPACE.to_string());
	}
	Ok(spec)
}

/// Write a local-phase spec to disk.
pub fn save_spec(id: &str, spec: &AppSpec) -> Result<PathBuf, SpecError> {
	let path = spec_path(id)?;
	let bytes = jsonx::marshal_indent(spec, "", "  ").map_err(SpecError::Json)?;
	write_atomic(&path, &bytes)?;
	Ok(path)
}

/// Remove the spec file. Missing files are not an error.
pub fn delete_spec(id: &str) -> Result<(), SpecError> {
	let path = spec_path(id)?;
	match fs::remove_file(&path) {
		Ok(()) => Ok(()),
		Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
		Err(e) => Err(SpecError::Io(e)),
	}
}

/// Load every local-phase spec in the directory, skipping files that fail to
/// parse.
pub fn load_all() -> Result<Vec<AppSpec>, SpecError> {
	let dir = get_spec_dir()?;
	let mut specs = Vec::new();
	for entry in fs::read_dir(&dir).map_err(SpecError::Io)? {
		let entry = match entry {
			Ok(e) => e,
			Err(_) => continue,
		};
		let path = entry.path();
		if entry.file_type().map(|t| t.is_dir()).unwrap_or(true) {
			continue;
		}
		if path.extension().and_then(|s| s.to_str()) != Some("json") {
			continue;
		}
		let bytes = match fs::read(&path) {
			Ok(b) => b,
			Err(_) => continue,
		};
		let mut spec: AppSpec = match jsonx::unmarshal(&bytes) {
			Ok(s) => s,
			Err(_) => continue,
		};
		if spec.namespace.as_deref().unwrap_or("").is_empty() {
			spec.namespace = Some(DEFAULT_NAMESPACE.to_string());
		}
		specs.push(spec);
	}
	Ok(specs)
}

// ---- Protocol-flavour I/O --------------------------------------------------
//
// These are what the daemon manager uses. Identical on-disk layout, same
// permissions, but the wire shape is `protocol::AppSpec`.

/// Save a `protocol::AppSpec` to disk. Mirrors the Go `spec.SaveSpec`.
pub fn save_spec_protocol(id: &str, spec: &ProtocolAppSpec) -> Result<PathBuf, SpecError> {
	let path = spec_path(id)?;
	let bytes = jsonx::marshal_indent(spec, "", "  ").map_err(SpecError::Json)?;
	write_atomic(&path, &bytes)?;
	Ok(path)
}

/// Load a `protocol::AppSpec` by ID.
pub fn load_spec_protocol(id: &str) -> Result<ProtocolAppSpec, SpecError> {
	let path = spec_path(id)?;
	let bytes = fs::read(&path).map_err(SpecError::Io)?;
	let mut spec: ProtocolAppSpec = jsonx::unmarshal(&bytes).map_err(SpecError::Json)?;
	if spec.namespace.as_deref().unwrap_or("").is_empty() {
		spec.namespace = Some(DEFAULT_NAMESPACE.to_string());
	}
	Ok(spec)
}

/// Delete a protocol spec by ID. Mirrors `spec.DeleteSpec`.
pub fn delete_spec_protocol(id: &str) -> Result<(), SpecError> {
	delete_spec(id)
}

/// Load every protocol spec in the directory, skipping files that fail to
/// parse. Mirrors `spec.LoadAll`.
pub fn load_all_protocol() -> Result<Vec<ProtocolAppSpec>, SpecError> {
	let dir = get_spec_dir()?;
	let mut specs = Vec::new();
	for entry in fs::read_dir(&dir).map_err(SpecError::Io)? {
		let entry = match entry {
			Ok(e) => e,
			Err(e) => {
				eprintln!("warning: read_dir entry: {e}");
				continue;
			}
		};
		let path = entry.path();
		if entry.file_type().map(|t| t.is_dir()).unwrap_or(true) {
			continue;
		}
		if path.extension().and_then(|s| s.to_str()) != Some("json") {
			continue;
		}
		let bytes = match fs::read(&path) {
			Ok(b) => b,
			Err(e) => {
				eprintln!("warning: read spec file {}: {e}", path.display());
				continue;
			}
		};
		let mut spec: ProtocolAppSpec = match jsonx::unmarshal(&bytes) {
			Ok(s) => s,
			Err(e) => {
				eprintln!("warning: parse spec file {}: {e}", path.display());
				continue;
			}
		};
		if spec.namespace.as_deref().unwrap_or("").is_empty() {
			spec.namespace = Some(DEFAULT_NAMESPACE.to_string());
		}
		specs.push(spec);
	}
	Ok(specs)
}

fn spec_path(id: &str) -> Result<PathBuf, SpecError> {
	let dir = get_spec_dir()?;
	Ok(dir.join(format!("{id}.json")))
}

fn write_atomic(path: &PathBuf, bytes: &[u8]) -> Result<(), SpecError> {
	fs::write(path, bytes).map_err(SpecError::Io)?;
	#[cfg(unix)]
	{
		use std::os::unix::fs::PermissionsExt;
		let _ = fs::set_permissions(path, fs::Permissions::from_mode(0o600));
	}
	Ok(())
}

/// Errors surfaced by the spec helpers.
#[derive(Debug)]
pub enum SpecError {
	Io(std::io::Error),
	Json(jsonx::Error),
	NoHome,
}

impl std::fmt::Display for SpecError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			SpecError::Io(e) => write!(f, "io error: {e}"),
			SpecError::Json(e) => write!(f, "json error: {e}"),
			SpecError::NoHome => f.write_str("could not get user home dir"),
		}
	}
}

impl std::error::Error for SpecError {}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::ipc::protocol::{AppExec as ProtocolAppExec, AppSpec as ProtocolAppSpec};

	#[test]
	fn get_spec_dir_creates_xdg_apps_with_0700() {
		let dir = tempfile::tempdir().expect("tempdir");
		std::env::set_var("XDG_CONFIG_HOME", dir.path());
		let path = get_spec_dir().expect("get_spec_dir");
		assert_eq!(path, dir.path().join("unitpm/apps"));
		let meta = fs::metadata(&path).expect("metadata");
		assert!(meta.is_dir());
		#[cfg(unix)]
		{
			use std::os::unix::fs::PermissionsExt;
			assert_eq!(meta.permissions().mode() & 0o777, 0o700);
		}
	}

	#[test]
	fn save_load_delete_round_trip() {
		let dir = tempfile::tempdir().expect("tempdir");
		std::env::set_var("XDG_CONFIG_HOME", dir.path());

		let id = "test-app-id";
		let spec = AppSpec {
			name: "test-app".into(),
			namespace: Some("test-ns".into()),
			exec: AppExec {
				command: Some("echo hello".into()),
			},
		};

		let path = save_spec(id, &spec).expect("save_spec");
		assert!(fs::metadata(&path).is_ok());

		let loaded = load_spec(id).expect("load_spec");
		assert_eq!(loaded.name, spec.name);
		assert_eq!(loaded.namespace, spec.namespace);

		let all = load_all().expect("load_all");
		assert_eq!(all.len(), 1);
		assert_eq!(all[0].name, spec.name);

		delete_spec(id).expect("delete_spec");
		assert!(fs::metadata(&path).is_err());
		assert!(load_spec(id).is_err());
	}

	#[test]
	fn generate_id_yields_unique_values() {
		let a = generate_id();
		let b = generate_id();
		assert!(!a.is_empty());
		assert_ne!(a, b);
	}

	#[test]
	fn protocol_spec_round_trip() {
		let dir = tempfile::tempdir().expect("tempdir");
		std::env::set_var("XDG_CONFIG_HOME", dir.path());

		let id = "protocol-app-id";
		let spec = ProtocolAppSpec {
			version: 1,
			id: id.into(),
			name: "protocol-app".into(),
			namespace: Some("test-ns".into()),
			exec: ProtocolAppExec {
				kind: "command".into(),
				command: Some("sleep 5".into()),
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
		};

		let path = save_spec_protocol(id, &spec).expect("save_spec_protocol");
		assert!(fs::metadata(&path).is_ok());

		let loaded = load_spec_protocol(id).expect("load_spec_protocol");
		assert_eq!(loaded.name, spec.name);
		assert_eq!(loaded.namespace.as_deref(), Some("test-ns"));

		let all = load_all_protocol().expect("load_all_protocol");
		assert_eq!(all.len(), 1);
		assert_eq!(all[0].name, spec.name);

		delete_spec_protocol(id).expect("delete_spec_protocol");
		assert!(fs::metadata(&path).is_err());
	}
}
