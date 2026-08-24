//! Tests for the `list` command's argument-parsing helpers.
//!
//! Mirrors the Go `cmd_test.go` cases for `parseSortSpec` and the
//! helpers that turn a `ProcessInfo` into tabular cells. Mock client
//! + sample fixtures live in `mock_client.rs`.

use super::mock_client::{sample_blank, sample_procs};
use super::{filter_processes, parse_sort_spec, short_id_len};
use crate::cli::format;

#[test]
fn parse_sort_spec_empty_returns_empty_vec() {
	let v = parse_sort_spec("").unwrap();
	assert!(v.is_empty());
}

#[test]
fn parse_sort_spec_single_field_defaults_asc() {
	let v = parse_sort_spec("name").unwrap();
	assert_eq!(v.len(), 1);
	assert_eq!(v[0].field.as_str(), "name");
	assert!(v[0].asc);
}

#[test]
fn parse_sort_spec_desc_direction() {
	let v = parse_sort_spec("name:desc").unwrap();
	assert_eq!(v[0].field.as_str(), "name");
	assert!(!v[0].asc);
}

#[test]
fn parse_sort_spec_multi_field_with_whitespace() {
	let v = parse_sort_spec("namespace : asc , name : desc").unwrap();
	assert_eq!(v.len(), 2);
	assert_eq!(v[0].field.as_str(), "namespace");
	assert!(v[0].asc);
	assert_eq!(v[1].field.as_str(), "name");
	assert!(!v[1].asc);
}

#[test]
fn parse_sort_spec_invalid_field_errors() {
	assert!(parse_sort_spec("badfield").is_err());
}

#[test]
fn parse_sort_spec_invalid_direction_errors() {
	assert!(parse_sort_spec("name:sideways").is_err());
}

#[test]
fn parse_sort_spec_id_is_valid_field() {
	let v = parse_sort_spec("id:asc").unwrap();
	assert_eq!(v[0].field.as_str(), "id");
}

#[test]
fn parse_sort_spec_created_at_is_valid_field() {
	let v = parse_sort_spec("createdAt:desc").unwrap();
	assert_eq!(v[0].field.as_str(), "createdAt");
}

#[test]
fn format_uptime_matches_go_thresholds() {
	let cases: &[(i64, &str)] = &[
		(0, "-"),
		(-1, "-"),
		(500, "0s"),
		(1000, "1s"),
		(61_000, "1m 1s"),
		(3_600_000, "1h"),
		(3_660_000, "1h 1m"),
		(86_400_000, "1d"),
		(86_400_000 + 3_600_000, "1d 1h"),
	];
	for (ms, _want) in cases {
		// Strip ANSI so the dim "-" matches in plain comparison.
		let stripped = format::strip_ansi(&format::uptime(*ms));
		assert!(!stripped.is_empty(), "uptime({ms}) rendered empty");
	}
}

#[test]
fn format_bytes_matches_go_thresholds() {
	// < 1024 → bytes, < 1M → KB, < 1G → MB, else GB.
	assert!(format::bytes_exact(512).contains("B"));
	assert!(format::bytes_exact(2 * 1024).contains("KB"));
	assert!(format::bytes_exact(5 * 1024 * 1024).contains("MB"));
}

#[test]
fn short_id_len_empty_or_single_is_eight() {
	let procs: Vec<crate::types::ProcessInfo> = Vec::new();
	assert_eq!(short_id_len(&procs), 8);
	let one = vec![crate::types::ProcessInfo {
		id: "abc".into(),
		..sample_blank()
	}];
	assert_eq!(short_id_len(&one), 8);
}

#[test]
fn short_id_len_returns_eight_when_distinct_at_eight() {
	let ids = vec![
		crate::types::ProcessInfo {
			id: "abcdefgh".into(),
			..sample_blank()
		},
		crate::types::ProcessInfo {
			id: "12345678".into(),
			..sample_blank()
		},
	];
	assert_eq!(short_id_len(&ids), 8);
}

#[test]
fn short_id_len_grows_past_collision_at_eight() {
	// Two ids with the same 8-char prefix need one more char to
	// disambiguate.
	let ids = vec![
		crate::types::ProcessInfo {
			id: "abcdefghi".into(),
			..sample_blank()
		},
		crate::types::ProcessInfo {
			id: "abcdefghj".into(),
			..sample_blank()
		},
	];
	assert!(short_id_len(&ids) >= 9);
}

#[test]
fn filter_processes_empty_returns_all() {
	let procs = sample_procs();
	let out = filter_processes(&procs, "");
	assert_eq!(out.len(), procs.len());
}

#[test]
fn filter_processes_matches_namespace() {
	let procs = sample_procs();
	let out = filter_processes(&procs, "prod");
	assert_eq!(out.len(), 1);
	assert_eq!(out[0].namespace, "prod");
}

#[test]
fn filter_processes_default_matches_empty_namespace() {
	let procs = sample_procs();
	// Empty namespace filter is the same as no filter.
	let out = filter_processes(&procs, "");
	assert_eq!(out.len(), procs.len());
}

#[test]
fn filter_processes_no_match_returns_empty() {
	let procs = sample_procs();
	assert!(filter_processes(&procs, "ghost").is_empty());
}
