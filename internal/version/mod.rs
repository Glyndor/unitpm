//! Build information and versioning.
//!
//! The four `pub static` fields are set at build time via `-ldflags` style
//! environment variables consumed by `build.rs` in a later phase. Phase 1
//! keeps the default values used by the Go `var` block so the test suite can
//! run without a custom build.

/// Semver string of the build.
pub static VERSION: &str = "0.13.1";

/// Git commit hash the binary was built from.
pub static COMMIT: &str = "none";

/// Build timestamp, free-form string.
pub static BUILD_DATE: &str = "unknown";

/// IPC protocol version, must match the protocol package's constant.
pub const PROTOCOL_VERSION: i64 = 1;

/// Serializable snapshot of the build info.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct Info {
	pub version: String,
	pub commit: String,
	#[serde(rename = "build_date")]
	pub build_date: String,
	#[serde(rename = "protocol_version")]
	pub protocol_version: i64,
}

/// Snapshot of the current build info.
#[must_use]
pub fn get() -> Info {
	Info {
		version: VERSION.to_string(),
		commit: COMMIT.to_string(),
		build_date: BUILD_DATE.to_string(),
		protocol_version: PROTOCOL_VERSION,
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn get_returns_info() {
		let info = get();
		assert!(!info.version.is_empty(), "version should not be empty");
		assert!(
			info.protocol_version > 0,
			"protocol version must be positive"
		);
	}

	#[test]
	fn get_default_values_are_semver_like() {
		let info = get();
		assert!(
			info.version.contains('.'),
			"version {} should contain a dot (semver)",
			info.version
		);
	}

	#[test]
	fn get_is_json_serializable() {
		let info = get();
		let bytes = serde_json::to_vec(&info).expect("marshal");
		let got: Info = serde_json::from_slice(&bytes).expect("unmarshal");
		assert_eq!(got.version, info.version);
		assert_eq!(got.protocol_version, info.protocol_version);
	}

	#[test]
	fn get_emits_all_info_fields_in_json() {
		let info = get();
		let bytes = serde_json::to_vec(&info).expect("marshal");
		let raw: serde_json::Value = serde_json::from_slice(&bytes).expect("unmarshal");
		for field in ["version", "commit", "build_date", "protocol_version"] {
			assert!(raw.get(field).is_some(), "JSON missing field {field:?}");
		}
	}

	#[test]
	fn protocol_version_constant_is_one() {
		// Matches the Go protocol package's constant.
		assert_eq!(get().protocol_version, 1);
	}
}
