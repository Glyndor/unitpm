//! Tests for the version command.
//!
//! 10 cases ported from `internal/cli/commands/version/cmd_test.go`.

use std::io;

use crate::cli::commands::version::{self, Ipc};
use crate::ipc::protocol::MismatchData;
use crate::ipc::transport::TransportError;
use crate::version::Info;

/// Mock Ipc. Records calls and replays a configured response or error.
/// `err` is wrapped in a Box so `version()` can hand it out without
/// consuming — multiple invocations see the same error.
struct MockIpc {
	response: Option<Info>,
	err: Option<Box<TransportError>>,
}

impl MockIpc {
	fn ok(response: Info) -> Self {
		Self {
			response: Some(response),
			err: None,
		}
	}

	fn err(err: TransportError) -> Self {
		Self {
			response: None,
			err: Some(Box::new(err)),
		}
	}
}

impl Ipc for MockIpc {
	fn version(&mut self) -> Result<Info, TransportError> {
		if let Some(e) = self.err.as_deref() {
			// Return a fresh value of the same variant each call so the
			// command body sees the same observable behaviour without us
			// needing to clone `TransportError`.
			return Err(rebuild_error(e));
		}
		Ok(self.response.clone().expect("configured response"))
	}
}

/// Rebuild an error of the same variant. Limited to the variants the
/// tests actually use; anything else panics so we notice if we need to
/// extend it.
fn rebuild_error(e: &TransportError) -> TransportError {
	match e {
		TransportError::Io(io) => TransportError::Io(io::Error::new(io.kind(), format!("{io}"))),
		TransportError::Remote(r) => TransportError::Remote(crate::ipc::protocol::RemoteError {
			code: r.code.clone(),
			message: r.message.clone(),
			data: r.data.clone(),
		}),
		_ => unreachable!("rebuild_error: variant {:?} not supported in tests", e),
	}
}

/// Every test that runs the version command must hold the term-state
/// lock so the `is_quiet()` check inside `run` is serialised with the
/// quiet-flipping test. Tests that simply inspect the spec don't need it.
fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

#[test]
fn run_no_daemon_prints_cli_section() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = version::run(None, &mut buf, &[]);
	rc.expect("run ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(out.contains("CLI"), "expected CLI section; got {out:?}");
	assert!(
		out.contains("Protocol"),
		"expected Protocol section; got {out:?}"
	);
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = version::run(None, &mut buf, &["--help".to_string()]);
	rc.expect("help ok");
}

#[test]
fn run_unexpected_positional_is_usage_error() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = version::run(None, &mut buf, &["arg1".to_string()]);
	let err = rc.expect_err("extra arg must error");
	assert!(
		err.to_string().contains("Unexpected arguments"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_daemon_success_includes_daemon_section() {
	let _g = lock_term();
	let info = Info {
		version: "0.4.10".into(),
		commit: "abc123".into(),
		build_date: "2026-04-14".into(),
		protocol_version: 1,
	};
	let client = MockIpc::ok(info);
	let mut buf = Vec::new();
	let rc = version::run(Some(Box::new(client)), &mut buf, &[]);
	rc.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(
		out.to_lowercase().contains("daemon"),
		"expected daemon section; got {out:?}"
	);
}

#[test]
fn run_protocol_mismatch_renders_mismatch_block() {
	let _g = lock_term();
	let remote_err = TransportError::Remote(crate::ipc::protocol::RemoteError {
		code: "PROTOCOL_MISMATCH".into(),
		message: "incompatible".into(),
		data: serde_json::to_value(MismatchData {
			supported: 2,
			received: 1,
		})
		.ok(),
	});
	let client = MockIpc::err(remote_err);
	let mut buf = Vec::new();
	let rc = version::run(Some(Box::new(client)), &mut buf, &[]);
	rc.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(
		out.contains("Protocol"),
		"expected Protocol section; got {out:?}"
	);
	assert!(
		out.to_lowercase().contains("mismatch"),
		"expected mismatch note; got {out:?}"
	);
}

#[test]
fn run_daemon_error_other_prints_protocol_section() {
	let _g = lock_term();
	let err = TransportError::Io(io::Error::new(io::ErrorKind::TimedOut, "timeout"));
	let client = MockIpc::err(err);
	let mut buf = Vec::new();
	let rc = version::run(Some(Box::new(client)), &mut buf, &[]);
	rc.expect("non-mismatch daemon error is benign");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(
		out.contains("Protocol"),
		"expected Protocol section; got {out:?}"
	);
}

#[test]
fn run_json_no_daemon_is_valid_json() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = version::run(None, &mut buf, &["--json".to_string()]);
	rc.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	let parsed: serde_json::Value = serde_json::from_str(&out).expect("json");
	assert!(
		parsed.get("cli").is_some(),
		"JSON missing 'cli'; got {parsed}"
	);
	assert!(
		parsed.get("protocol").is_some(),
		"JSON missing 'protocol'; got {parsed}"
	);
}

#[test]
fn run_json_with_daemon_includes_daemon() {
	let _g = lock_term();
	let info = Info {
		version: "0.4.10".into(),
		commit: "abc123".into(),
		build_date: "2026-04-14".into(),
		protocol_version: 1,
	};
	let client = MockIpc::ok(info);
	let mut buf = Vec::new();
	let rc = version::run(Some(Box::new(client)), &mut buf, &["--json".to_string()]);
	rc.expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	let parsed: serde_json::Value = serde_json::from_str(&out).expect("json");
	assert!(
		parsed.get("daemon").is_some(),
		"JSON missing 'daemon'; got {parsed}"
	);
}

#[test]
fn run_quiet_silences_human_block() {
	// Quiet suppresses everything; the buffer stays empty.
	let _g = lock_term();
	crate::term::set_quiet(true);
	let mut buf = Vec::new();
	let rc = version::run(None, &mut buf, &[]);
	rc.expect("ok");
	assert!(
		buf.is_empty(),
		"quiet mode should produce no output; got {:?}",
		String::from_utf8_lossy(&buf)
	);
}

#[test]
fn get_spec_matches_name() {
	let s = version::spec();
	assert_eq!(s.name, "version");
	assert!(
		s.options.iter().any(|o| o.long == "--json"),
		"version spec missing --json"
	);
	assert!(
		s.options.iter().any(|o| o.long == "--help"),
		"version spec missing --help"
	);
}
