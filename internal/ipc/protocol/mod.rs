//! IPC protocol types and constants.
//!
//! Defines the wire format for the daemon-client protocol: the request and
//! response envelopes, the start-command specific payload, the application
//! specification, and the error shapes returned over the socket. The Go
//! counterpart lives at `internal/ipc/protocol` and the wire format is
//! preserved exactly so that the daemon and CLI can interoperate.

use serde::{Deserialize, Serialize};

use crate::jsonx;

/// Wire status: the daemon rejected the request.
pub const STATUS_ERROR: &str = "error";
/// Wire status: the daemon accepted the request.
pub const STATUS_SUCCESS: &str = "success";

/// Protocol version understood by both ends. Bumped on any breaking change
/// to the request/response envelopes.
pub const VERSION: i32 = 1;

/// Standard IPC request envelope.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Request {
	pub version: i32,
	pub id: String,
	pub command: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub params: Option<RawMessage>,
}

/// Standard IPC response envelope.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Response {
	pub version: i32,
	pub id: String,
	pub status: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub result: Option<RawMessage>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub error: Option<Box<Error>>,
}

/// Request payload for the `start` command.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct StartRequest {
	pub protocol_version: i32,
	pub request_id: String,
	#[serde(rename = "type")]
	pub kind: String,
	pub spec: AppSpec,
}

/// Response for a `start` request.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct StartResponse {
	pub protocol_version: i32,
	#[serde(rename = "type")]
	pub kind: String,
	pub request_id: String,
	pub ok: bool,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub data: Option<Box<StartResponseData>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub error: Option<Box<StartError>>,
}

/// Success payload of a `start` response.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct StartResponseData {
	pub id: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub proc_id: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub pid: Option<i32>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub status: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub message: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub created_at: Option<String>,
}

/// Error payload of a `start` response.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct StartError {
	pub code: String,
	pub message: String,
}

/// Full specification of an application to be run.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppSpec {
	pub version: i32,
	pub id: String,
	pub name: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub namespace: Option<String>,
	pub exec: AppExec,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub cwd: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub env: Option<std::collections::BTreeMap<String, String>>,
	#[serde(default, rename = "envFile", skip_serializing_if = "Option::is_none")]
	pub env_file: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub logs: Option<Box<AppLogs>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub restart: Option<Box<AppRestart>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub cron: Option<String>,
	#[serde(default, rename = "runAs", skip_serializing_if = "Option::is_none")]
	pub run_as: Option<Box<RunAsPolicy>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub stop: Option<Box<AppStop>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub resources: Option<Box<AppResources>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub watch: Option<Box<AppWatch>>,
	#[serde(default, rename = "createdAt", skip_serializing_if = "Option::is_none")]
	pub created_at: Option<String>,
	#[serde(default, skip_serializing_if = "is_false")]
	pub disabled: bool,
}

/// Controls how the process is terminated. Zero values use sensible defaults
/// (SIGTERM, 10s grace period).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppStop {
	/// Signal name to deliver first. `SIGTERM` if empty.
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub signal: Option<String>,
	/// Time to wait after the first signal before sending SIGKILL. Bounded to
	/// `[1000, 300000]`.
	#[serde(default, rename = "timeoutMs", skip_serializing_if = "Option::is_none")]
	pub timeout_ms: Option<i32>,
}

/// Payload returned by the `scale` IPC verb.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ScaleResponse {
	#[serde(rename = "base_name")]
	pub base_name: String,
	pub namespace: String,
	pub before: i32,
	pub after: i32,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub created: Option<Vec<String>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub deleted: Option<Vec<String>>,
}

/// Filesystem watching configuration.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppWatch {
	pub enabled: bool,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub ignore: Option<Vec<String>>,
}

/// Runtime resource bounds.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppResources {
	#[serde(
		default,
		rename = "memory_max_bytes",
		skip_serializing_if = "Option::is_none"
	)]
	pub memory_max_bytes: Option<i64>,
	#[serde(
		default,
		rename = "cpu_max_percent",
		skip_serializing_if = "Option::is_none"
	)]
	pub cpu_max_percent: Option<i32>,
	#[serde(default, rename = "tasks_max", skip_serializing_if = "Option::is_none")]
	pub tasks_max: Option<i32>,
}

/// Execution details.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppExec {
	#[serde(rename = "type")]
	pub kind: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub command: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub args: Option<Vec<String>>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub entry: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub runtime: Option<String>,
	#[serde(default, skip_serializing_if = "is_false")]
	pub shell: bool,
}

/// Logging configuration.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppLogs {
	pub mode: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub dir: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub stdout: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub stderr: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub format: Option<String>,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub timestamp: Option<String>,
}

/// Restart policy.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AppRestart {
	pub policy: String,
	#[serde(
		default,
		rename = "maxRetries",
		skip_serializing_if = "Option::is_none"
	)]
	pub max_retries: Option<i32>,
	#[serde(default, rename = "backoffMs", skip_serializing_if = "Option::is_none")]
	pub backoff_ms: Option<i32>,
	#[serde(
		default,
		rename = "backoffType",
		skip_serializing_if = "Option::is_none"
	)]
	pub backoff_type: Option<String>,
	#[serde(
		default,
		rename = "stopOnExit",
		skip_serializing_if = "Option::is_none"
	)]
	pub stop_on_exit: Option<Vec<i32>>,
}

/// Isolation/user settings.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RunAsPolicy {
	pub mode: String,
}

/// Structured error returned inside a [`Response`].
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Error {
	pub code: String,
	pub message: String,
	#[serde(default, skip_serializing_if = "Option::is_none")]
	pub data: Option<serde_json::Value>,
}

/// Wraps an IPC error response on the client side. Implements [`std::error::Error`]
/// so `?` on a `Result<_, RemoteError>` surfaces the wire error directly.
#[derive(Debug, Clone, PartialEq)]
pub struct RemoteError {
	pub code: String,
	pub message: String,
	pub data: Option<serde_json::Value>,
}

impl std::fmt::Display for RemoteError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "ipc error: [{}] {}", self.code, self.message)
	}
}

impl std::error::Error for RemoteError {}

/// Carries the protocol-version mismatch context inside an [`Error::data`] payload.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct MismatchData {
	pub supported: i32,
	pub received: i32,
}

/// Internal alias for `jsonx::RawMessage`, kept here so call sites do not need
/// to import the jsonx module directly.
pub type RawMessage = jsonx::RawMessage;

fn is_false(b: &bool) -> bool {
	!*b
}

#[cfg(all(test, target_os = "linux"))]
mod tests;
