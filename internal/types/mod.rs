//! Shared type definitions.
//!
//! These types are referenced across multiple modules in the daemon and the
//! CLI. Keeping them in one place avoids circular package dependencies.

use serde::{Deserialize, Serialize};

/// Namespace assigned to specs that do not set one.
pub const DEFAULT_NAMESPACE: &str = "default";

/// State a managed process can be in.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum ProcessState {
	Running,
	Online,
	Stopped,
	Failed,
	Exited,
	Restarting,
}

impl ProcessState {
	/// Wire-format string, matching the JSON the Go implementation emits.
	pub const fn as_str(self) -> &'static str {
		match self {
			ProcessState::Running => "running",
			ProcessState::Online => "online",
			ProcessState::Stopped => "stopped",
			ProcessState::Failed => "failed",
			ProcessState::Exited => "exited",
			ProcessState::Restarting => "restarting",
		}
	}
}

/// Summary of a process's state and configuration.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ProcessInfo {
	pub id: String,
	pub name: String,
	pub namespace: String,
	pub version: String,
	pub mode: String,
	pub pid: i64,
	#[serde(rename = "uptime_ms")]
	pub uptime: i64,
	pub restarts: i64,
	pub state: ProcessState,
	#[serde(rename = "cpu")]
	pub cpu: f64,
	#[serde(rename = "memory_bytes")]
	pub memory: i64,
	pub user: String,
	pub watch: bool,
	#[serde(rename = "git_branch", skip_serializing_if = "Option::is_none")]
	pub git_branch: Option<String>,
	#[serde(rename = "git_commit", skip_serializing_if = "Option::is_none")]
	pub git_commit: Option<String>,
	#[serde(rename = "git_dirty", skip_serializing_if = "is_false")]
	pub git_dirty: bool,
	#[serde(rename = "created_at", skip_serializing_if = "Option::is_none")]
	pub created_at: Option<String>,
}

fn is_false(b: &bool) -> bool {
	!*b
}

#[cfg(test)]
mod tests {
	use super::*;
	use serde_json;

	#[test]
	fn process_state_constants_match_wire_strings() {
		let cases = [
			(ProcessState::Running, "running"),
			(ProcessState::Online, "online"),
			(ProcessState::Stopped, "stopped"),
			(ProcessState::Failed, "failed"),
			(ProcessState::Exited, "exited"),
			(ProcessState::Restarting, "restarting"),
		];
		for (got, want) in cases {
			assert_eq!(got.as_str(), want, "ProcessState::as_str mismatch");
		}
		assert_eq!(DEFAULT_NAMESPACE, "default");
	}

	#[test]
	fn process_info_marshal_round_trip() {
		let input = ProcessInfo {
			id: "p1".into(),
			name: "api".into(),
			namespace: "ns".into(),
			version: "1.0".into(),
			mode: "fork".into(),
			pid: 1234,
			uptime: 5000,
			restarts: 2,
			state: ProcessState::Online,
			cpu: 12.5,
			memory: 1024,
			user: "root".into(),
			watch: true,
			git_branch: Some("main".into()),
			git_commit: Some("abc".into()),
			git_dirty: true,
			created_at: Some("2024-01-01".into()),
		};
		let bytes = serde_json::to_vec(&input).expect("marshal");
		let output: ProcessInfo = serde_json::from_slice(&bytes).expect("unmarshal");
		assert_eq!(output, input);
	}

	#[test]
	fn process_info_omits_empty_git_and_created_at() {
		let info = ProcessInfo {
			id: "p".into(),
			name: "x".into(),
			namespace: "default".into(),
			version: String::new(),
			mode: String::new(),
			pid: 0,
			uptime: 0,
			restarts: 0,
			state: ProcessState::Running,
			cpu: 0.0,
			memory: 0,
			user: String::new(),
			watch: false,
			git_branch: None,
			git_commit: None,
			git_dirty: false,
			created_at: None,
		};
		let bytes = serde_json::to_vec(&info).expect("marshal");
		let s = std::str::from_utf8(&bytes).expect("utf8");
		for k in ["git_branch", "git_commit", "git_dirty", "created_at"] {
			assert!(!s.contains(k), "expected {k} omitted, got {s}");
		}
		for k in ["\"id\"", "\"pid\"", "\"uptime_ms\"", "\"memory_bytes\""] {
			assert!(s.contains(k), "expected {k} present, got {s}");
		}
	}
}
