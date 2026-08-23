//! Tests for the updater.
//!
//! Ports the 16 cases from `updater_test.go`, 5 from `cache_test.go`, plus
//! the refusal cases the brief required. The Go suite's refusal coverage is
//! partial — it covers the no-`.sig` path but not the ed25519 mismatch path
//! — so the mismatch cases are net additions.
//!
//! Each test that swaps the release URL or the cache path takes the test
//! mutex and restores on `Drop`. The URL override is process-global, the
//! cache path is read from env vars — without the guard, a panicking test
//! would leak the override to the next one.

use std::path::PathBuf;
use std::time::Duration;

use base64::Engine;
use ed25519_dalek::{Signer, SigningKey};

use super::*;
use crate::updater::test_server::{build_response, json_body, json_response, TestServer};

pub(crate) fn setup_release_server(release: &Release, status: u16) -> (TestServer, UrlGuard) {
	let body = if status == 200 {
		json_body(release)
	} else {
		Vec::new()
	};
	let server = TestServer::new(vec![json_response(status, &body)]);
	let guard = UrlGuard::new(&server.url("/releases/latest"));
	(server, guard)
}

/// A release whose tag matches the running version — `check` returns None.
pub(crate) fn up_to_date_release() -> Release {
	Release {
		tag_name: format!("v{}", crate::version::VERSION),
		..Release::default()
	}
}

// --- Tests ----------------------------------------------------------------

#[test]
fn http_get_ok() {
	let server = TestServer::new(vec![build_response(
		200,
		"application/octet-stream",
		b"hello",
	)]);
	let body = http_get(&server.url("/x"), Duration::from_secs(2), 0).expect("get");
	assert_eq!(body, b"hello");
}

#[test]
fn http_get_limited() {
	let server = TestServer::new(vec![build_response(
		200,
		"application/octet-stream",
		b"0123456789abcdef",
	)]);
	let body = http_get(&server.url("/x"), Duration::from_secs(2), 8).expect("get");
	assert_eq!(body.len(), 8);
}

#[test]
fn http_get_404() {
	let server = TestServer::new(vec![build_response(404, "text/plain", b"nope")]);
	let err = http_get(&server.url("/x"), Duration::from_secs(2), 0).expect_err("404");
	assert!(matches!(err, Error::Http(s) if s.contains("404")));
}

#[test]
fn http_get_queue_exhausted_returns_500() {
	// No canned responses — every request should get the definite 500, never
	// hang. This is the safety net the harness exists to provide.
	let server = TestServer::new(Vec::new());
	let err = http_get(&server.url("/x"), Duration::from_secs(2), 0).expect_err("500");
	assert!(matches!(err, Error::Http(s) if s.contains("500")));
}

#[test]
fn check_up_to_date() {
	let release = up_to_date_release();
	let (_s, _g) = setup_release_server(&release, 200);
	let r = check().expect("check");
	assert!(r.is_none(), "expected nil, got {r:?}");
}

#[test]
fn check_older_available_no_downgrade() {
	let release = Release {
		tag_name: "v0.0.1".into(),
		..Release::default()
	};
	let (_s, _g) = setup_release_server(&release, 200);
	let r = check().expect("check");
	assert!(r.is_none(), "older release must not surface: {r:?}");
}

#[test]
fn check_newer_available() {
	let release = Release {
		tag_name: "v99.99.99".into(),
		assets: vec![Asset {
			name: asset_basename(),
			browser_download_url: "https://example.com/bin".into(),
		}],
		html_url: "https://example.com/r".into(),
		..Release::default()
	};
	let (_s, _g) = setup_release_server(&release, 200);
	let r = check().expect("check").expect("release");
	assert_eq!(r.tag_name, "v99.99.99");
	assert_eq!(r.assets.len(), 1);
}

#[test]
fn check_http_error() {
	let release = Release::default();
	let (_s, _g) = setup_release_server(&release, 500);
	let err = check().expect_err("500");
	assert!(matches!(err, Error::Http(_)));
}

#[test]
fn check_bad_json() {
	let server = TestServer::new(vec![build_response(200, "application/json", b"not json")]);
	let _g = UrlGuard::new(&server.url("/releases/latest"));
	let err = check().expect_err("bad json");
	assert!(matches!(err, Error::Json(_)));
}

#[test]
fn is_newer_table() {
	let cases = [
		("1.0.0", "0.9.9", true),
		("0.9.9", "1.0.0", false),
		("1.2.3", "1.2.3", false),
		("1.0.1", "1.0.0", true),
		("2.0.0", "1.99.99", true),
		("1.10.0", "1.2.0", true),
	];
	for (a, b, want) in cases {
		assert_eq!(super::is_newer(a, b), want, "is_newer({a}, {b})");
	}
}

#[test]
fn parse_version_table() {
	let cases = [
		("1.2.3", [1, 2, 3]),
		("0.4.11", [0, 4, 11]),
		("1.0", [1, 0, 0]),
		("abc", [0, 0, 0]),
	];
	for (in_, want) in cases {
		assert_eq!(super::parse_version(in_), want, "parse_version({in_})");
	}
}

#[test]
fn is_managed_by_package_system_no_panic() {
	let _ = is_managed_by_package_system();
}

// Depth tests for `is_managed_by_package_system` live in `is_managed_tests`.

#[test]
fn apply_no_compatible_binary() {
	let (_dir, exe) = exe_fixture();
	let release = Release {
		tag_name: "v99.0.0".into(),
		assets: vec![Asset {
			name: "irrelevant-asset".into(),
			browser_download_url: "https://example.com/x".into(),
		}],
		..Release::default()
	};
	let err = apply_to_path(
		&exe,
		&release,
		ApplyOptions {
			allow_unsigned: true,
		},
	)
	.expect_err("expected error");
	let msg = err.to_string();
	assert!(msg.contains("no compatible binary"), "got: {msg}");
}

#[test]
fn apply_missing_signature_requires_flag() {
	let (_dir, exe) = exe_fixture();
	let release = Release {
		tag_name: "v99.0.0".into(),
		assets: vec![Asset {
			name: asset_basename(),
			browser_download_url: "https://127.0.0.1:1/bin".into(),
		}],
		..Release::default()
	};
	let err = apply_to_path(
		&exe,
		&release,
		ApplyOptions {
			allow_unsigned: false,
		},
	)
	.expect_err("expected refusal");
	assert!(err.is_signature_required(), "got: {err}");
}

#[test]
fn apply_allow_unsigned_bypasses_sig_check() {
	let (_dir, exe) = exe_fixture();
	let release = Release {
		tag_name: "v99.0.0".into(),
		assets: vec![Asset {
			name: asset_basename(),
			browser_download_url: "https://127.0.0.1:1/bin".into(),
		}],
		..Release::default()
	};
	let err = apply_to_path(
		&exe,
		&release,
		ApplyOptions {
			allow_unsigned: true,
		},
	)
	.expect_err("network error");
	assert!(
		!err.is_signature_required(),
		"AllowUnsigned must skip SigRequired, got {err}"
	);
}

// --- decode_signature -----------------------------------------------------

#[test]
fn decode_signature_raw_bytes() {
	let sig = [42u8; 64];
	let got = decode_signature(&sig).expect("raw bytes decode");
	assert_eq!(got, sig);
}

#[test]
fn decode_signature_base64_standard() {
	let sig = [7u8; 64];
	let encoded = base64::engine::general_purpose::STANDARD.encode(sig);
	let got = decode_signature(encoded.as_bytes()).expect("b64 std");
	assert_eq!(got, sig);
}

#[test]
fn decode_signature_malformed() {
	let err = decode_signature(b"not-a-signature").expect_err("malformed");
	assert!(matches!(err, keys::SigError::Malformed { bytes: 15 }));
}

// --- verify_signature -----------------------------------------------------

fn signing_key() -> SigningKey {
	let mut bytes = [0u8; 32];
	for (i, b) in bytes.iter_mut().enumerate() {
		*b = (i as u8).wrapping_add(1);
	}
	SigningKey::from_bytes(&bytes)
}

#[test]
fn verify_signature_valid() {
	let sk = signing_key();
	let vk = sk.verifying_key();
	let body = b"binary content for testing";
	let sig = sk.sign(body);
	let sig_bytes: [u8; 64] = sig.to_bytes();
	assert!(verify_signature(&vk, body, &sig_bytes));
}

#[test]
fn verify_signature_invalid_key() {
	let sk1 = signing_key();
	let mut other = [0u8; 32];
	for (i, b) in other.iter_mut().enumerate() {
		*b = (i as u8).wrapping_add(99);
	}
	let sk2 = SigningKey::from_bytes(&other);
	let body = b"binary content for testing";
	let sig = sk2.sign(body);
	let sig_bytes: [u8; 64] = sig.to_bytes();
	assert!(!verify_signature(&sk1.verifying_key(), body, &sig_bytes));
}

#[test]
fn verify_signature_valid_for_different_bytes() {
	let sk = signing_key();
	let vk = sk.verifying_key();
	let body_a = b"actual binary";
	let body_b = b"something else";
	let sig = sk.sign(body_a);
	let sig_bytes: [u8; 64] = sig.to_bytes();
	// Signature is valid for body_a but not body_b.
	assert!(!verify_signature(&vk, body_b, &sig_bytes));
}

// --- apply_to_path end-to-end with a real signature ----------------------

fn end_to_end_apply(
	content: &[u8],
	sig: &[u8; 64],
	opts: ApplyOptions,
	pub_key: &ed25519_dalek::VerifyingKey,
) -> Result<(), Error> {
	let dir = tempfile::tempdir().expect("tempdir");
	let exe = dir.path().join("unitpm");
	std::fs::write(&exe, b"old").expect("write");
	let sig_b64 = base64::engine::general_purpose::STANDARD.encode(sig);

	let server = TestServer::new(vec![
		build_response(200, "application/octet-stream", content),
		build_response(200, "text/plain", sig_b64.as_bytes()),
	]);
	let asset = server.url("/bin");
	let sig_asset = server.url("/sig");
	let release = Release {
		tag_name: "v99.0.0".into(),
		assets: vec![
			Asset {
				name: asset_basename(),
				browser_download_url: asset,
			},
			Asset {
				name: format!("{}.sig", asset_basename()),
				browser_download_url: sig_asset,
			},
		],
		body: String::new(),
		html_url: String::new(),
	};
	apply_to_path_with_key(&exe, &release, opts, pub_key)
}

/// Stub exe path used by the apply_* tests so the rename at the end of
/// `download_and_replace` has somewhere real to land. Returns the
/// tempdir so the caller can keep it alive across the apply call.
fn exe_fixture() -> (tempfile::TempDir, PathBuf) {
	let dir = tempfile::tempdir().expect("tempdir");
	let exe = dir.path().join("unitpm");
	std::fs::write(&exe, b"old").expect("write");
	(dir, exe)
}

#[test]
fn apply_accepted_with_valid_signature() {
	let sk = signing_key();
	let body = b"new binary content";
	let sig: [u8; 64] = sk.sign(body).to_bytes();
	let result = end_to_end_apply(
		body,
		&sig,
		ApplyOptions {
			allow_unsigned: false,
		},
		&sk.verifying_key(),
	);
	assert!(
		result.is_ok(),
		"apply with valid signature must succeed, got {result:?}"
	);
}

#[test]
fn apply_refused_signature_does_not_match() {
	let sk = signing_key();
	let body = b"new binary content";
	let bad_sig: [u8; 64] = [1u8; 64];
	let result = end_to_end_apply(
		body,
		&bad_sig,
		ApplyOptions {
			allow_unsigned: false,
		},
		&sk.verifying_key(),
	);
	let err = result.expect_err("must refuse");
	assert!(matches!(err, Error::SignatureInvalid), "got {err}");
}

#[test]
fn apply_refused_signature_for_different_bytes() {
	let sk = signing_key();
	let body_downloaded = b"downloaded bytes";
	let body_signed = b"different bytes";
	let sig: [u8; 64] = sk.sign(body_signed).to_bytes();
	let result = end_to_end_apply(
		body_downloaded,
		&sig,
		ApplyOptions {
			allow_unsigned: false,
		},
		&sk.verifying_key(),
	);
	let err = result.expect_err("must refuse");
	assert!(matches!(err, Error::SignatureInvalid), "got {err}");
}

#[test]
fn apply_refused_no_signature_asset() {
	let (_dir, exe) = exe_fixture();
	let server = TestServer::new(vec![build_response(
		200,
		"application/octet-stream",
		b"new binary",
	)]);
	let asset = server.url("/bin");
	let release = Release {
		tag_name: "v99.0.0".into(),
		assets: vec![Asset {
			name: asset_basename(),
			browser_download_url: asset,
		}],
		..Release::default()
	};
	let err = apply_to_path(
		&exe,
		&release,
		ApplyOptions {
			allow_unsigned: false,
		},
	)
	.expect_err("must refuse");
	assert!(err.is_signature_required(), "got: {err}");
}
