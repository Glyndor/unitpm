//! End-to-end tests for the `list` command's runtime path.
//!
//! The argument-parsing tests live in `parser_tests.rs`, and the MockClient
//! and its sample fixtures in `mock_client.rs`. The tests here cover the
//! surface that drives an actual IPC round trip against a canned response:
//! `run`, `--json`, the render highlight, `fetch_and_render` and
//! `wait_update_and_notify`.

use std::collections::HashSet;
use std::time::Instant;

use crate::cli::commands::list::{
	fetch_and_render, render_to, run, wait_update_and_notify, RenderOptions,
};
use crate::types::{ProcessInfo, ProcessState};
use crate::updater;

use super::mock_client::{empty_procs, sample_blank, sample_procs, MockClient};

#[test]
fn run_help_writes_no_error() {
	let mut client = MockClient::new(empty_procs());
	let mut out = Vec::new();
	let args = vec!["--help".to_string()];
	let err = run(&mut client, &mut out, &args);
	assert!(err.is_ok(), "--help should not error");
}

#[test]
fn run_unknown_flag_errors() {
	let mut client = MockClient::new(empty_procs());
	let mut out = Vec::new();
	let args = vec!["--badFlag".to_string()];
	let err = run(&mut client, &mut out, &args);
	assert!(err.is_err(), "expected unknown-flag error");
}

#[test]
fn run_unexpected_positional_errors() {
	let mut client = MockClient::new(empty_procs());
	let mut out = Vec::new();
	let args = vec!["unexpected".to_string()];
	let err = run(&mut client, &mut out, &args);
	assert!(err.is_err(), "expected unexpected-args error");
}

#[test]
fn run_ipc_error_surfaces_with_list_failed_prefix() {
	let mut client = MockClient::err("connection refused");
	let mut out = Vec::new();
	let args: Vec<String> = Vec::new();
	let result = run(&mut client, &mut out, &args);
	let msg = match result {
		Ok(_) => String::new(),
		Err(e) => e.to_string(),
	};
	assert!(msg.contains("list failed"), "got {msg:?}");
}

#[test]
fn run_with_empty_list_makes_one_ipc_call() {
	let mut client = MockClient::new(empty_procs());
	let mut out = Vec::new();
	let args: Vec<String> = Vec::new();
	run(&mut client, &mut out, &args).expect("ok");
	assert_eq!(client.list_calls(), vec!["list".to_string()]);
}

#[test]
fn run_with_processes_renders_without_error() {
	let mut client = MockClient::new(sample_procs());
	let mut out = Vec::new();
	let args: Vec<String> = Vec::new();
	run(&mut client, &mut out, &args).expect("ok");
	assert!(!out.is_empty(), "expected some rendered output");
}

#[test]
fn run_with_namespace_filter_keeps_only_match() {
	let mut client = MockClient::new(sample_procs());
	let mut out = Vec::new();
	let args = vec!["--namespace".to_string(), "prod".to_string()];
	run(&mut client, &mut out, &args).expect("ok");
	let plain = crate::cli::format::strip_ansi(&String::from_utf8_lossy(&out));
	assert!(plain.contains("z-app"), "expected z-app from prod: {plain}");
	assert!(
		!plain.contains("a-app"),
		"did not expect staging row: {plain}"
	);
}

#[test]
fn run_with_long_flag_does_not_error() {
	let mut client = MockClient::new(sample_procs());
	let mut out = Vec::new();
	let args = vec!["--long".to_string()];
	run(&mut client, &mut out, &args).expect("ok");
	// The Go test pins the `Run` call surface only; the table is
	// wrap-aware at narrow widths, so the full UUID does not appear as a
	// single cell. Asserting on no-error matches the Go coverage.
	assert!(!out.is_empty());
}

#[test]
fn run_with_sort_flag_does_not_error() {
	let mut client = MockClient::new(sample_procs());
	let mut out = Vec::new();
	let args = vec!["--sort".to_string(), "name:asc".to_string()];
	run(&mut client, &mut out, &args).expect("ok");
}

#[test]
fn run_with_invalid_sort_returns_error() {
	let mut client = MockClient::new(empty_procs());
	let mut out = Vec::new();
	let args = vec!["--sort".to_string(), "badfield".to_string()];
	let err = run(&mut client, &mut out, &args);
	assert!(err.is_err(), "expected invalid-sort error");
}

// --- --json ----------------------------------------------------------------

#[test]
fn run_json_serialises_processes() {
	let mut client = MockClient::new(sample_procs());
	let mut out = Vec::new();
	let args = vec!["--json".to_string()];
	run(&mut client, &mut out, &args).expect("ok");
	let got = String::from_utf8_lossy(&out);
	let parsed: Vec<ProcessInfo> = serde_json::from_str(got.trim()).expect("valid json");
	assert_eq!(parsed.len(), 2);
	assert_eq!(parsed[0].name, "z-app");
	assert_eq!(parsed[1].name, "a-app");
}

#[test]
fn run_json_empty_serialises_to_empty_array() {
	let mut client = MockClient::new(empty_procs());
	let mut out = Vec::new();
	let args = vec!["--json".to_string()];
	run(&mut client, &mut out, &args).expect("ok");
	let got = String::from_utf8_lossy(&out);
	let parsed: Vec<ProcessInfo> = serde_json::from_str(got.trim()).expect("valid json");
	assert!(parsed.is_empty());
}

// --- render highlight ------------------------------------------------------

#[test]
fn render_highlight_by_id_marks_target_row() {
	let mut out = Vec::new();
	let procs = vec![
		ProcessInfo {
			id: "aaaaaaaa-0000-0000-0000-000000000000".into(),
			name: "api".into(),
			namespace: "prod".into(),
			state: ProcessState::Running,
			..sample_blank()
		},
		ProcessInfo {
			id: "bbbbbbbb-0000-0000-0000-000000000000".into(),
			name: "worker".into(),
			namespace: "prod".into(),
			state: ProcessState::Running,
			..sample_blank()
		},
	];
	let mut highlight = HashSet::new();
	highlight.insert("aaaaaaaa-0000-0000-0000-000000000000".to_string());
	let opts = RenderOptions {
		highlight,
		..RenderOptions::default()
	};
	render_to(&mut out, &procs, &opts);
	let plain = crate::cli::format::strip_ansi(&String::from_utf8_lossy(&out));
	assert!(
		plain.contains("▸"),
		"expected highlight marker ▸, got:\n{plain}"
	);
	let marker_idx = plain.find("▸").unwrap();
	let worker_idx = plain.find("worker").unwrap();
	assert!(marker_idx < worker_idx, "marker on api row");
	let count = plain.matches("▸").count();
	assert_eq!(count, 1, "expected exactly one ▸, got {count}:\n{plain}");
}

#[test]
fn render_highlight_by_name_marks_target_row() {
	let mut out = Vec::new();
	let procs = vec![ProcessInfo {
		id: "aaa".into(),
		name: "api".into(),
		namespace: "prod".into(),
		state: ProcessState::Running,
		..sample_blank()
	}];
	let mut highlight = HashSet::new();
	highlight.insert("api".to_string());
	let opts = RenderOptions {
		highlight,
		..RenderOptions::default()
	};
	render_to(&mut out, &procs, &opts);
	let plain = crate::cli::format::strip_ansi(&String::from_utf8_lossy(&out));
	assert!(plain.contains("▸"), "expected ▸ when highlighting by name");
}

#[test]
fn render_without_highlight_emits_no_marker() {
	let mut out = Vec::new();
	let procs = vec![ProcessInfo {
		id: "aaa".into(),
		name: "api".into(),
		namespace: "prod".into(),
		state: ProcessState::Running,
		..sample_blank()
	}];
	render_to(&mut out, &procs, &RenderOptions::default());
	let plain = crate::cli::format::strip_ansi(&String::from_utf8_lossy(&out));
	assert!(
		!plain.contains("▸"),
		"no highlight requested, marker must not appear"
	);
}

// --- fetch_and_render (silently swallows IPC errors) ----------------------

#[test]
fn fetch_and_render_swallows_ipc_error() {
	let mut out = Vec::new();
	let mut client = MockClient::err("daemon offline");
	let mut highlight = HashSet::new();
	highlight.insert("x".into());
	fetch_and_render(&mut client, highlight, &mut out);
	assert!(
		out.is_empty(),
		"ipc error must not leak to out: {:?}",
		String::from_utf8_lossy(&out)
	);
}

#[test]
fn fetch_and_render_empty_list_does_not_panic() {
	let mut out = Vec::new();
	let mut client = MockClient::new(empty_procs());
	let highlight = HashSet::new();
	fetch_and_render(&mut client, highlight, &mut out);
	// Empty list renders headers/borders only; we do not assert on
	// content beyond "did not panic".
}

// --- wait_update_and_notify ----------------------------------------------

#[test]
fn wait_update_and_notify_nil_release_is_noop() {
	let (tx, rx) = std::sync::mpsc::channel();
	tx.send(None).unwrap();
	let deadline = Instant::now() + std::time::Duration::from_millis(100);
	wait_update_and_notify(&rx, deadline);
}

#[test]
fn wait_update_and_notify_with_release_does_not_panic() {
	let (tx, rx) = std::sync::mpsc::channel();
	let release = updater::CachedRelease {
		tag_name: "v1.2.3".into(),
		html_url: String::new(),
	};
	tx.send(Some(release)).unwrap();
	let deadline = Instant::now() + std::time::Duration::from_millis(100);
	wait_update_and_notify(&rx, deadline);
}

#[test]
fn wait_update_and_notify_timeout_returns_immediately() {
	let (_tx, rx) = std::sync::mpsc::channel::<Option<updater::CachedRelease>>();
	let deadline = Instant::now() - std::time::Duration::from_secs(1);
	wait_update_and_notify(&rx, deadline);
}

#[test]
fn print_update_banner_does_not_panic() {
	// The Go TestPrintUpdateBanner_NoPanel calls printUpdateBanner directly;
	// here we route through the module's private helper via the public
	// sibling, which is what Go's `printUpdateBanner` ends up doing under
	// `--json` suppression.
	let release = updater::CachedRelease {
		tag_name: "v9.9.9".into(),
		html_url: String::new(),
	};
	let (tx, rx) = std::sync::mpsc::channel();
	tx.send(Some(release)).unwrap();
	let deadline = Instant::now() + std::time::Duration::from_millis(100);
	wait_update_and_notify(&rx, deadline);
}
